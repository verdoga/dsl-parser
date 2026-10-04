package parser

import (
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestLinesRouteEditorContentAndPreserveSource(t *testing.T) {
	source := "\uFEFF@dsl-version 1.2\r\n@editor @unknown\n@editor {\r\n@unknown\n@document-id Hidden\n# Hidden\n\\}\n \t\r\n}\n@document-id Visible\n尾🌍"
	p := inputParser(t, source)
	if err := p.Parse(); err != nil {
		t.Fatal(err)
	}
	result := p.result
	if len(result.Diagnostics) != 0 || result.Document.HasErrors || result.Document.Metadata.Title != nil || result.Document.Metadata.DocumentID == nil || *result.Document.Metadata.DocumentID != "Visible" {
		t.Fatalf("Содержимое повторно интерпретировано: %+v", result.Diagnostics)
	}
	if result.Lines[1].LineType != model.LineTypeTag || len(result.Lines[1].Elements) != 2 || *result.Lines[1].Elements[1].Value != "@unknown" {
		t.Fatal("Хвост однострочного editor потерян")
	}
	for _, i := range []int{3, 4, 5, 6} {
		line := result.Lines[i]
		if line.LineType != model.LineTypeContent || len(line.Elements) != 1 || line.Elements[0].ElementType != model.ElementTypeContent || line.ParentLine == nil || *line.ParentLine != 3 {
			t.Fatalf("Неверный маршрут содержимого: %+v", line)
		}
	}
	if result.Lines[7].LineType != model.LineTypeBlank || result.Lines[8].LineType != model.LineTypeBlockEnd || result.Lines[8].ParentLine == nil || *result.Lines[8].ParentLine != 3 || result.Lines[9].ParentLine != nil {
		t.Fatal("Пустая строка или закрытие блока обработаны неверно")
	}
	var restored strings.Builder
	for _, line := range result.Lines {
		restored.WriteString(line.Raw)
		restored.WriteString(string(line.LineEnding))
		if line.Elements == nil {
			t.Fatal("Elements=null")
		}
		for _, element := range line.Elements {
			if element.ErrorIDs == nil {
				t.Fatal("ErrorIDs=null")
			}
		}
	}
	if restored.String() != strings.TrimPrefix(source, "\uFEFF") || *result.Document.LineCount != 11 {
		t.Fatal("Изменены исходные строки или EOL")
	}
}

func TestUnfinishedLinesPreserveParsedElementsAndContinue(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		retained int
		kind     model.LineType
	}{
		{"  #Ошибка🌍  ", 0, model.LineTypeInvalid},
		{`@media audio "Незавершённый🌍`, 2, model.LineTypeTag},
		{`@resource-dir Known, "Незавершённый`, 2, model.LineTypeTag},
		{`@resource-dir "Bad"x, Known`, 2, model.LineTypeTag},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			p := inputParser(t, "@dsl-version 1.2\n"+tc.raw+"\n@document-id Next\nText")
			if err := p.Parse(); err != nil {
				t.Fatal(err)
			}
			line := p.result.Lines[1]
			if line.Raw != tc.raw || line.LineType != tc.kind || !line.HasErrors || len(p.result.Lines) != 4 || p.result.Document.Metadata.DocumentID == nil || *p.result.Document.Metadata.DocumentID != "Next" {
				t.Fatal("Незавершённая строка остановила обработку или утратила тип")
			}
			retained, unfinished := 0, 0
			for _, element := range line.Elements {
				if element.ElementType != model.ElementTypeUnparsed {
					retained++
					continue
				}
				unfinished++
				if element.Value != nil || len(element.ErrorIDs) != 1 {
					t.Fatal("У unparsed нет однозначной связи с P015")
				}
				found := false
				for _, diagnostic := range p.result.Diagnostics {
					if diagnostic.ID != element.ErrorIDs[0] {
						continue
					}
					found = diagnostic.DiagnosticCode == diagnostics.P015 && !diagnostic.Fatal && diagnostic.DiagnosticScope == diagnostics.ScopeElement && diagnostic.Location != nil && diagnostic.Location.Start == (model.Position{Line: 2, Column: element.Start}) && diagnostic.Location.End == (model.Position{Line: 2, Column: element.End})
				}
				if !found {
					t.Fatal("Диапазон P015 не совпадает с unparsed")
				}
			}
			if retained != tc.retained || unfinished == 0 || p.result.Lines[2].HasErrors || p.result.Lines[3].HasErrors {
				t.Fatal("Распознанные элементы утрачены или ошибка перенесена на следующие строки")
			}
			if err := checkElementPositions(line); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestElementPositionsUseUnicodeAndRejectOverlaps(t *testing.T) {
	for _, tc := range []struct {
		name     string
		elements []model.Element
		valid    bool
	}{
		{"unicode", []model.Element{{Raw: "я", Start: 1, End: 2}, {Raw: "🌍", Start: 2, End: 3}}, true},
		{"insertion", []model.Element{{Start: 1, End: 1}, {Raw: "я🌍", Start: 1, End: 3}, {Start: 3, End: 3}}, true},
		{"zero", []model.Element{{Start: 0, End: 0}}, false},
		{"reversed", []model.Element{{Start: 2, End: 1}}, false},
		{"bytes", []model.Element{{Raw: "я", Start: 1, End: 3}}, false},
		{"out of bounds", []model.Element{{Start: 3, End: 4}}, false},
		{"raw mismatch", []model.Element{{Raw: "x", Start: 1, End: 2}}, false},
		{"overlap", []model.Element{{Raw: "я🌍", Start: 1, End: 3}, {Raw: "🌍", Start: 2, End: 3}}, false},
		{"wrong order", []model.Element{{Raw: "🌍", Start: 2, End: 3}, {Raw: "я", Start: 1, End: 2}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := model.Line{Number: 1, Raw: "я🌍", Elements: tc.elements}
			before := parserSnapshot(t, line)
			if err := checkElementPositions(line); (err == nil) != tc.valid {
				t.Fatalf("Проверка координат: %v", err)
			}
			if parserSnapshot(t, line) != before {
				t.Fatal("Проверка изменила элементы")
			}
		})
	}
}

func TestAppendLineKeepsSourceAndOwnsSlices(t *testing.T) {
	p := inputParser(t, "")
	source := model.Line{Number: 1, Raw: "я", LineEnding: model.LineEndingCRLF}
	parsed := model.Line{Number: 99, Raw: "wrong", LineType: model.LineTypeContent, Elements: []model.Element{{Raw: "я", Start: 1, End: 2, ErrorIDs: []string{"id"}}}}
	if err := p.appendLine(source, parsed); err != nil {
		t.Fatal(err)
	}
	parsed.Elements[0].ErrorIDs[0], parsed.Elements[0].Raw = "changed", "changed"
	got := p.result.Lines[0]
	if got.Number != 1 || got.Raw != "я" || got.LineEnding != model.LineEndingCRLF || got.Elements[0].Raw != "я" || got.Elements[0].ErrorIDs[0] != "id" {
		t.Fatal("Исходные данные или сохранённые элементы изменены")
	}
	if err := p.appendLine(model.Line{Number: 2}, model.Line{}); err != nil || p.result.Lines[1].Elements == nil {
		t.Fatal("Не создан пустой массив элементов")
	}
	before := parserSnapshot(t, p.result)
	if err := p.appendLine(model.Line{Number: 4}, model.Line{}); err == nil || parserSnapshot(t, p.result) != before {
		t.Fatal("Нарушение порядка строк не отклонено")
	}
}

func TestLineRoutesCallRegisteredFunctionsInOrder(t *testing.T) {
	p := inputParser(t, "@dsl-version 1.2\n@editor {\n@unknown\n}\nText")
	registered, _ := p.grammars.Lookup("1.2")
	var calls []string
	detect := registered.DetectionFuncs()[0]
	orchestrate := registered.OrchestrationFuncs()[0]
	enter, leave := registered.Assertions()
	p.grammars.Register("1.2", []grammar.DetectionFunc{
		func(c grammar.GrammarContext) bool { calls = append(calls, "d1"); return detect(c) },
		func(c grammar.GrammarContext) bool { calls = append(calls, "d2"); return true },
	}, []grammar.OrchestrationFunc{
		func(c grammar.GrammarContext) bool {
			calls = append(calls, "o1")
			if c.Line().Number == 3 && (c.Line().LineType != model.LineTypeContent || c.Line().ParentLine != nil) {
				t.Fatal("Не сохранён заранее заданный тип содержимого")
			}
			return orchestrate(c)
		},
		func(c grammar.GrammarContext) bool { calls = append(calls, "o2"); return true },
	}, nil, registered.LineParserFuncs(), enter, leave)
	if err := p.Parse(); err != nil {
		t.Fatal(err)
	}
	want := []string{"d1", "d2", "o1", "o2", "d1", "d2", "o1", "o2", "d1", "d2", "o1", "o2", "d1", "d2", "o1", "o2", "d1", "d2", "o1", "o2"}
	if !slices.Equal(calls, want) {
		t.Fatalf("Порядок вызовов: %v", calls)
	}
}

func TestLineUpdateSeesSavedMetadataAndDiagnostics(t *testing.T) {
	p := inputParser(t, "@dsl-version 1.2\n@section Kept\n@unknown")
	registered, _ := p.grammars.Lookup("1.2")
	enter, _ := registered.Assertions()
	check := p.assertionFuncs["1.2"][enter]
	var numbers []int
	p.assertionFuncs["1.2"][enter] = func(input grammar.AssertionInput) bool {
		numbers = append(numbers, input.Line.Number)
		if len(p.result.Lines) != input.Line.Number {
			t.Fatal("Стек обновляется до сохранения строки")
		}
		if input.Line.Number >= 2 && (p.result.Document.Metadata.Section == nil || *p.result.Document.Metadata.Section != "Kept") {
			t.Fatal("Метаданные ещё не собраны")
		}
		if input.Line.Number == 3 && (len(p.result.Diagnostics) != 1 || len(input.Line.Elements[0].ErrorIDs) != 1) {
			t.Fatal("Диагностика ещё не связана с элементом")
		}
		return check(input)
	}
	if err := p.Parse(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(numbers, []int{1, 2, 3}) {
		t.Fatalf("Порядок строк: %v", numbers)
	}
}

func TestFailedDetectionAndEmptyRemainderProduceP015(t *testing.T) {
	for _, empty := range []bool{false, true} {
		p := nestingParser(t)
		p.detections = []grammar.DetectionFunc{func(c grammar.GrammarContext) bool {
			if empty {
				c.AddElement(model.Element{ElementType: model.ElementTypeContent, Raw: "я", Start: 1, End: 2})
			}
			return false
		}}
		p.orchestrators = []grammar.OrchestrationFunc{func(grammar.GrammarContext) bool {
			t.Fatal("Оркестрация после неуспешной детекции")
			return true
		}}
		context, err := p.parseLine(model.Line{Number: 1, Raw: "я"})
		if err != nil {
			t.Fatal(err)
		}
		line := context.Line()
		if err := p.importLineDiagnostics(context, &line); err != nil {
			t.Fatal(err)
		}
		last := line.Elements[len(line.Elements)-1]
		if last.ElementType != model.ElementTypeUnparsed || len(last.ErrorIDs) != 1 || (empty && (last.Start != 2 || last.End != 2 || last.Raw != "")) || (!empty && last.Raw != "я") {
			t.Fatal("Неверный диапазон незавершённого разбора")
		}
	}
}

func TestMalformedClosingResetsStacksAndContinuesParsing(t *testing.T) {
	p := inputParser(t, "@dsl-version 1.2\n@variants {\n@task A\n@note {\n} }\n@task B\nText\n@endtask\n}")
	if err := p.Parse(); err != nil {
		t.Fatal(err)
	}
	if len(p.result.Lines) != 9 {
		t.Fatal("Потеряны строки")
	}
	for _, diagnostic := range p.result.Diagnostics {
		if diagnostic.Fatal {
			t.Fatalf("Локальная ошибка стала фатальной: %+v", diagnostic)
		}
	}
	if len(p.result.Diagnostics) != 2 || p.result.Diagnostics[0].DiagnosticCode != diagnostics.P010 || p.result.Diagnostics[1].DiagnosticCode != diagnostics.P009 {
		t.Fatalf("Диагностики: %+v", p.result.Diagnostics)
	}
	for _, i := range []int{4, 5, 7, 8} {
		if p.result.Lines[i].ParentLine != nil || p.result.Lines[i].NestingLevel != 0 {
			t.Fatalf("Строка %d не корневая", i+1)
		}
	}
	next := p.result.Lines[6]
	if next.ParentLine == nil || *next.ParentLine != 6 || next.NestingLevel != 1 {
		t.Fatal("После сброса не создано новое задание")
	}
	if p.result.Lines[2].ParentLine == nil || *p.result.Lines[2].ParentLine != 2 {
		t.Fatal("Переписаны прежние связи")
	}
}

func TestRepeatedClosingDiagnosticStillResetsStacks(t *testing.T) {
	p := inputParser(t, "@dsl-version 1.2\n@variants {\n} }\n@task New\nText")
	p.result.Diagnostics = append(p.result.Diagnostics, model.Diagnostic{
		ID: "existing", Source: p.processingID, DiagnosticCode: diagnostics.P010,
		SeverityLevel: diagnostics.SeverityError, DiagnosticScope: diagnostics.ScopeLine,
		Location: &model.Location{Start: model.Position{Line: 3, Column: 1}, End: model.Position{Line: 3, Column: 4}},
	})
	if err := p.Parse(); err != nil {
		t.Fatal(err)
	}
	if len(p.result.Diagnostics) != 1 || p.result.Diagnostics[0].ID != "existing" {
		t.Fatalf("Повтор диагностики: %+v", p.result.Diagnostics)
	}
	if p.result.Lines[2].ParentLine != nil || p.result.Lines[3].ParentLine != nil {
		t.Fatal("Повторная P010 не сбросила стеки")
	}
	if parent := p.result.Lines[4].ParentLine; parent == nil || *parent != 4 {
		t.Fatal("После сброса не создан новый родитель")
	}
}
