package parser

import (
	"crypto/sha256"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestContentBraceDiagnosticsHaveElementLinksAndParsingContinues(t *testing.T) {
	for _, opening := range []string{"", "@text {\n", "@multifill {\n", "@editor {\n"} {
		t.Run(opening, func(t *testing.T) {
			source := "@dsl-version 1.2\n" + opening + "  Ёж🌍 {x}  \n"
			contentIndex := 1
			if opening != "" {
				source += "}\n"
				contentIndex = 2
			}
			source += "@document-id Next\nText"
			p := inputParser(t, source)
			if err := p.Parse(); err != nil {
				t.Fatal(err)
			}
			if len(p.result.Diagnostics) != 1 {
				t.Fatalf("Диагностики: %+v", p.result.Diagnostics)
			}
			d := p.result.Diagnostics[0]
			line := p.result.Lines[contentIndex]
			if d.DiagnosticCode != diagnostics.P012 || d.Fatal || !p.result.Document.HasErrors || !line.HasErrors || len(line.Elements) != 1 {
				t.Fatalf("Неверная P012: %+v; строка %+v", d, line)
			}
			e := line.Elements[0]
			if len(e.ErrorIDs) != 1 || e.ErrorIDs[0] != d.ID || d.Location == nil || d.Location.Start.Line != line.Number || d.Location.Start.Column != e.Start || d.Location.End.Column != e.End {
				t.Fatal("Нарушена обратная ссылка P012")
			}
			if p.result.Document.Metadata.DocumentID == nil || *p.result.Document.Metadata.DocumentID != "Next" {
				t.Fatal("Разбор остановился после локальной ошибки")
			}
		})
	}
}

func TestPlaceholderSyntaxDoesNotValidatePlacement(t *testing.T) {
	for _, opening := range []string{"", "@multifill {\n", "@text {\n", "@editor {\n"} {
		source := "@dsl-version 1.2\n" + opening + `She _____{lives} and \{writes\}.` + "\n"
		if opening != "" {
			source += "}\n"
		}
		p := inputParser(t, source)
		if err := p.Parse(); err != nil {
			t.Fatal(err)
		}
		if len(p.result.Diagnostics) != 0 {
			t.Fatalf("Грамматика проверила размещение вместо синтаксиса: %+v", p.result.Diagnostics)
		}
	}
}

func TestMultifillEmptyLinesRemainBlank(t *testing.T) {
	p := inputParser(t, "@dsl-version 1.2\n@multifill {\n\n \t\n}")
	if err := p.Parse(); err != nil {
		t.Fatal(err)
	}
	for _, line := range p.result.Lines[2:4] {
		if line.LineType != model.LineTypeBlank || line.ParentLine == nil || *line.ParentLine != 2 || line.Elements == nil || len(line.Elements) != 0 {
			t.Fatalf("Пустая строка должна оставаться blank: %+v", line)
		}
	}
	if len(p.result.Diagnostics) != 0 {
		t.Fatalf("Лишние диагностики: %+v", p.result.Diagnostics)
	}
}

func TestMultifillReviewRegression(t *testing.T) {
	source, err := os.ReadFile("testdata/multifill-review.txt")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(source)) != "c34526f6448535ab0a767ed7ee4b6ef75fb71247452097233f9496100b96d600" {
		t.Fatal("Изменены байты исходного Review.txt")
	}
	result := assertContentFilterDifference(t, string(source), 51, 53, 58, 60, 62)
	if len(result.Diagnostics) != 0 || result.Document.HasErrors {
		t.Fatalf("Ложные ошибки Review.txt: %+v", result.Diagnostics)
	}
	var restored strings.Builder
	for _, line := range result.Lines {
		restored.WriteString(line.Raw)
		restored.WriteString(string(line.LineEnding))
		if line.HasErrors {
			t.Fatalf("Остался признак ошибки строки %d", line.Number)
		}
		for _, element := range line.Elements {
			if element.ErrorIDs == nil || len(element.ErrorIDs) != 0 {
				t.Fatalf("Осталась ссылка или null errorIds: %+v", element)
			}
		}
	}
	if restored.String() != string(source) {
		t.Fatal("Изменён исходный текст или окончания строк")
	}
}

func TestMultifillAlternativesRespectBlockBoundaries(t *testing.T) {
	for _, opening := range []string{"@multifill {", "@multifill Complete the text. {", "@MuLtIfIlL Complete the text. {"} {
		t.Run(opening, func(t *testing.T) {
			source := "@dsl-version 1.2\r\n_____{Before}{Outside}\n@task A\n" + opening +
				"\r\n  Ёж🌍 _____{Да}{Нет}{Может быть} и _____{X}{Y}  \r\n" +
				"_____{Single}\n\n \t\r\n}\n_____{After}{Outside}"
			result := assertContentFilterDifference(t, source, 5)
			if len(result.Diagnostics) != 2 || result.Diagnostics[0].Location.Start.Line != 2 || result.Diagnostics[1].Location.Start.Line != 10 {
				t.Fatalf("Изменены границы подавления: %+v", result.Diagnostics)
			}
			for _, number := range []int{5, 6, 7, 8, 9} {
				line := result.Lines[number-1]
				if line.ParentLine == nil || *line.ParentLine != 4 || line.NestingLevel != 2 {
					t.Fatalf("Потерян родитель блока: %+v", line)
				}
			}
			if result.Lines[6].LineType != model.LineTypeBlank || result.Lines[7].LineType != model.LineTypeBlank || result.Lines[9].ParentLine == nil || *result.Lines[9].ParentLine != 3 {
				t.Fatal("Неверная пустая строка или родитель после блока")
			}
		})
	}
}

func TestMultifillFilterPreservesRealErrorsAndOtherContexts(t *testing.T) {
	for _, content := range []string{
		`_____{A}{}`, `_____{A}{B`, `_____{A}{B{C}}`, `_____{A}}`,
		`_____{A} {B}`, `_____{A}{B} текст {`, `_____{A}{B} текст }`,
		`{ошибка} _____{A}{B}`, `_____{A}{B} _____{C}{}`, `Просто {скобки}`,
	} {
		t.Run(content, func(t *testing.T) {
			result := assertContentFilterDifference(t, "@dsl-version 1.2\n@multifill {\n"+content+"\n}")
			if len(result.Diagnostics) != 1 || result.Diagnostics[0].DiagnosticCode != diagnostics.P012 {
				t.Fatalf("Потеряна реальная P012: %+v", result.Diagnostics)
			}
		})
	}
	for _, tag := range []string{"text", "editor", "instr", "hint", "answer", "example", "wordlist", "note", "alt", "table", "script", "key"} {
		t.Run(tag, func(t *testing.T) {
			result := assertContentFilterDifference(t, "@dsl-version 1.2\n@"+tag+" {\n_____{A}{B}\n}")
			if len(result.Diagnostics) != 1 || result.Diagnostics[0].DiagnosticCode != diagnostics.P012 {
				t.Fatalf("Подавлена P012 вне multifill: %+v", result.Diagnostics)
			}
		})
	}
	for _, opening := range []string{"@multifill", "@multifill Write your answer."} {
		t.Run(opening, func(t *testing.T) {
			result := assertContentFilterDifference(t, "@dsl-version 1.2\n"+opening+"\n_____{A}{B}")
			if len(result.Diagnostics) != 1 || result.Diagnostics[0].DiagnosticCode != diagnostics.P012 || result.Lines[2].ParentLine != nil {
				t.Fatalf("Однострочный multifill стал блоком: %+v", result.Diagnostics)
			}
		})
	}
}

func TestMultifillFilterPreservesRecoveryAndSpecialOperands(t *testing.T) {
	for _, tc := range []struct {
		name, tail string
		codes      []diagnostics.Code
	}{
		{"bad closing", "} extra\n_____{Outside}{Still outside}", []diagnostics.Code{diagnostics.P010, diagnostics.P012}},
		{"unclosed", "", []diagnostics.Code{diagnostics.P011}},
		{"unparsed operand", "}\n@media audio \"Незавершённый", []diagnostics.Code{diagnostics.P015}},
		{"special operands", "}\n@media audio \"file{A}{B}.mp3\"\n@resource-dir Dir{A}{B}", nil},
		{"invalid media form", "}\n@media audio file{A}{B}.mp3", []diagnostics.Code{diagnostics.P005, diagnostics.P004}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := assertContentFilterDifference(t, "@dsl-version 1.2\n@multifill {\n_____{A}{B}\n"+tc.tail, 3)
			var codes []diagnostics.Code
			for _, d := range result.Diagnostics {
				codes = append(codes, d.DiagnosticCode)
			}
			if !slices.Equal(codes, tc.codes) {
				t.Fatalf("Изменены ошибки восстановления: %v, ожидаются %v", codes, tc.codes)
			}
			if tc.name == "bad closing" && (result.Lines[3].ParentLine != nil || result.Lines[4].ParentLine != nil || result.Lines[4].NestingLevel != 0) {
				t.Fatal("P010 не сбросила стеки")
			}
		})
	}
}

func TestMultifillTextDoesNotReinterpretDSL(t *testing.T) {
	source := "@dsl-version 1.2\n@multifill {\n" +
		"@document-id Hidden _____{A}{B}\n# Hidden _____{A}{B}\n@text _____{A}{B}\n" +
		"@multifill _____{A}{B}\n" + `\} _____{A}{B} и \{текст\}` +
		"\n}\n@document-id Visible"
	result := assertContentFilterDifference(t, source, 3, 4, 5, 6, 7)
	if len(result.Diagnostics) != 0 || result.Document.Metadata.Title != nil || result.Document.Metadata.DocumentID == nil || *result.Document.Metadata.DocumentID != "Visible" {
		t.Fatalf("Текст изменил метаданные или получил ошибки: %+v", result)
	}
	for _, line := range result.Lines[2:7] {
		if line.LineType != model.LineTypeContent || len(line.Elements) != 1 || line.Elements[0].ElementType != model.ElementTypeContent || line.ParentLine == nil || *line.ParentLine != 2 {
			t.Fatalf("DSL внутри текста интерпретирован повторно: %+v", line)
		}
	}
}

func TestMultifillSuppressionKeepsGrammarDiagnosticWithoutConsumingID(t *testing.T) {
	p := inputParser(t, "@dsl-version 1.2\n@multifill {\n_____{A}{B}\nBad {brace}\n}")
	registered, _ := p.grammars.Lookup("1.2")
	orchestrate := registered.OrchestrationFuncs()[0]
	enter, leave := registered.Assertions()
	var captured grammar.GrammarContext
	var before string
	p.grammars.Register("1.2", registered.DetectionFuncs(), []grammar.OrchestrationFunc{func(c grammar.GrammarContext) bool {
		ok := orchestrate(c)
		if c.Line().Number == 3 {
			captured, before = c, parserSnapshot(t, c.Diagnostics())
		}
		return ok
	}}, nil, registered.LineParserFuncs(), enter, leave)
	if err := p.Parse(); err != nil {
		t.Fatal(err)
	}
	if captured == nil || len(captured.Diagnostics()) != 1 || captured.Diagnostics()[0].DiagnosticCode != diagnostics.P012 || captured.Diagnostics()[0].ID != "" || parserSnapshot(t, captured.Diagnostics()) != before {
		t.Fatal("Исходная P012 в контексте грамматики изменена или удалена")
	}
	if len(p.result.Diagnostics) != 1 || p.result.Diagnostics[0].ID != "d1" || p.result.Diagnostics[0].Location.Start.Line != 4 || len(p.result.Lines[2].Elements[0].ErrorIDs) != 0 || !slices.Equal(p.result.Lines[3].Elements[0].ErrorIDs, []string{"d1"}) {
		t.Fatalf("Подавление израсходовало ID или нарушило ссылки: %+v", p.result.Diagnostics)
	}
}

// Сравниваем полные модели, разрешая только удаление перечисленных P012,
// соответствующих ссылок/флагов и сдвиг ID следующих диагностик.
func assertContentFilterDifference(t *testing.T, source string, suppressedLines ...int) *model.Result {
	t.Helper()
	baseline, filtered := inputParser(t, source), inputParser(t, source)
	baseline.diagnosticRules = nil
	for _, p := range []*Parser{baseline, filtered} {
		if err := p.Parse(); err != nil {
			t.Fatal(err)
		}
	}
	want := baseline.result
	ids := make(map[string]string)
	kept := make([]model.Diagnostic, 0, len(want.Diagnostics))
	var removed []int
	for _, d := range want.Diagnostics {
		if d.Location != nil && slices.Contains(suppressedLines, d.Location.Start.Line) {
			if d.DiagnosticCode != diagnostics.P012 {
				t.Fatalf("Вместо исходной P012 получена %+v", d)
			}
			removed = append(removed, d.Location.Start.Line)
			continue
		}
		ids[d.ID] = ""
		kept = append(kept, d)
	}
	if !slices.Equal(removed, suppressedLines) {
		t.Fatalf("Исходные P012 на строках %v, ожидаются %v", removed, suppressedLines)
	}
	// ID назначаются при импорте, а итоговый массив сортируется по координатам.
	// Перенумерация должна сохранить порядок назначения, а не порядок массива.
	nextID := 1
	for i := 1; i <= len(want.Diagnostics); i++ {
		old := fmt.Sprintf("d%d", i)
		if _, retained := ids[old]; retained {
			ids[old] = fmt.Sprintf("d%d", nextID)
			nextID++
		}
	}
	for i := range kept {
		kept[i].ID = ids[kept[i].ID]
	}
	want.Diagnostics = kept
	want.Document.HasErrors = len(kept) > 0
	for i := range want.Lines {
		line := &want.Lines[i]
		if slices.Contains(suppressedLines, line.Number) {
			line.HasErrors = false
		}
		for j := range line.Elements {
			element := &line.Elements[j]
			links := make([]string, 0, len(element.ErrorIDs))
			for _, old := range element.ErrorIDs {
				if id, retained := ids[old]; retained {
					links = append(links, id)
				}
			}
			element.ErrorIDs = links
		}
	}
	if parserSnapshot(t, filtered.result) != parserSnapshot(t, want) {
		t.Fatalf("Кроме ожидаемого удаления диагностик изменилась модель\nполучено: %s\nожидается: %s", parserSnapshot(t, filtered.result), parserSnapshot(t, want))
	}
	return filtered.result
}
