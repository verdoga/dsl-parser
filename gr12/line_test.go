package gr12

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestClassifyLineUsesOnlyPhysicalSyntax(t *testing.T) {
	for _, test := range []struct {
		source string
		kind   model.LineType
	}{
		{"", model.LineTypeBlank}, {" \t \t", model.LineTypeBlank},
		{"# Unit 1", model.LineTypeHeading}, {"## Vocabulary", model.LineTypeHeading},
		{" \t### Step 1. New words \t", model.LineTypeHeading},
		{"#Title", model.LineTypeInvalid}, {"#### Title", model.LineTypeInvalid},
		{"#", model.LineTypeInvalid}, {"## \t", model.LineTypeInvalid},
		{"###\tTitle", model.LineTypeInvalid}, {"#\u00a0Title", model.LineTypeInvalid},
		{"@note Text", model.LineTypeTag}, {"@note{", model.LineTypeTag},
		{"@unknown", model.LineTypeTag}, {"@", model.LineTypeTag},
		{"}", model.LineTypeBlockEnd}, {" \t} @note Text", model.LineTypeBlockEnd},
		{"{", model.LineTypeInvalid}, {" \t{ \t", model.LineTypeInvalid},
		{"Introductory text.", model.LineTypeContent}, {"---", model.LineTypeContent},
		{`\@note This is ordinary text.`, model.LineTypeContent},
		{`\# This is not a heading.`, model.LineTypeContent},
		{`\}`, model.LineTypeContent}, {`\\@note Text`, model.LineTypeContent},
		{"Use {x} here.", model.LineTypeContent}, {"{x}", model.LineTypeContent},
		{"\u00a0", model.LineTypeContent}, {"\u00a0@note Text", model.LineTypeContent},
	} {
		t.Run(test.source, func(t *testing.T) {
			context := newLineTestContext(t, test.source)
			before := context.Line()
			if !classifyLine(context) || context.Line().LineType != test.kind {
				t.Fatalf("Тип %s, требуется %s", context.Line().LineType, test.kind)
			}
			assertLineServiceFields(t, context, before)
			if len(context.Line().Elements) != 0 || len(context.Diagnostics()) != 0 {
				t.Fatal("Классификация выполнила разбор элементов или добавила диагностики")
			}
		})
	}
}

func TestParseLineHeadingsAndTagsFromDSL(t *testing.T) {
	for _, test := range []struct {
		source   string
		kind     model.LineType
		elements []model.Element
	}{
		// DSL 2.1: все три синтаксических уровня, независимо от глубины парсера.
		{"# Unit 1", model.LineTypeHeading, []model.Element{operandWant(model.ElementTypeHeadingLevel, "#", "1", 1), operandWant(model.ElementTypeTitle, "Unit 1", "Unit 1", 3)}},
		{"## Vocabulary", model.LineTypeHeading, []model.Element{operandWant(model.ElementTypeHeadingLevel, "##", "2", 1), operandWant(model.ElementTypeTitle, "Vocabulary", "Vocabulary", 4)}},
		{"### Step 1. New words", model.LineTypeHeading, []model.Element{operandWant(model.ElementTypeHeadingLevel, "###", "3", 1), operandWant(model.ElementTypeTitle, "Step 1. New words", "Step 1. New words", 5)}},
		{" \t###  Ёж🌍\tА \t", model.LineTypeHeading, []model.Element{operandWant(model.ElementTypeHeadingLevel, "###", "3", 3), operandWant(model.ElementTypeTitle, "Ёж🌍\tА", "Ёж🌍\tА", 8)}},
		{`# Use \{ and \} as text.`, model.LineTypeHeading, []model.Element{operandWant(model.ElementTypeHeadingLevel, "#", "1", 1), operandWant(model.ElementTypeTitle, `Use \{ and \} as text.`, `Use \{ and \} as text.`, 3)}},
		{"# Title @note Text", model.LineTypeHeading, []model.Element{operandWant(model.ElementTypeHeadingLevel, "#", "1", 1), operandWant(model.ElementTypeTitle, "Title @note Text", "Title @note Text", 3)}},
		// DSL 1.2, 3.9: первоначальный тип tag уточняется обработчиком объявления.
		{"@dsl-version 1.2", model.LineTypeTag, []model.Element{operandWant(model.ElementTypeTag, "@dsl-version", "dsl-version", 1), operandWant(model.ElementTypeVersion, "1.2", "1.2", 14)}},
		{"@note Tip {", model.LineTypeBlockStart, []model.Element{operandWant(model.ElementTypeTag, "@note", "note", 1), operandWant(model.ElementTypeTitle, "Tip", "Tip", 7), operandWant(model.ElementTypeBlockOpen, "{", "{", 11)}},
		{"", model.LineTypeBlank, nil}, {" \t", model.LineTypeBlank, nil},
	} {
		t.Run(test.source, func(t *testing.T) {
			context := newLineTestContext(t, test.source)
			before := context.Line()
			if !classifyLine(context) || !parseLine(context) {
				t.Fatal("Локальный разбор не завершён")
			}
			if context.Line().LineType != test.kind || len(context.Line().Elements) != len(test.elements) || len(context.Diagnostics()) != 0 {
				t.Fatalf("Неверный результат: %+v, диагностики %+v", context.Line(), context.Diagnostics())
			}
			for i, element := range test.elements {
				assertOperandElement(t, context.Line().Elements[i], element)
			}
			assertLineServiceFields(t, context, before)
		})
	}
}

func TestParseContentPreservesWholeTextWithoutInterpretation(t *testing.T) {
	// DSL 1.21, 3.4, 3.6, 4.3: экранирование, ячейки, HTML и плейсхолдеры.
	for _, text := range []string{
		`\@note This is ordinary text.`, `\# This is not a heading.`,
		`Use \{ and \} as text.`, `Literal underscores: \_____`,
		"Pronoun | Verb | Short form", "<strong><em>Important information</em></strong>",
		"<br>", "She _____ in London.", "She _____{lives} in London.",
		"---", "Italian; Spanish; British.", "Ёж🌍\tА",
		`\{`, `\}`, "\u00a0Text\u00a0",
	} {
		t.Run(text, func(t *testing.T) {
			context := newLineTestContext(t, " \t"+text+" \t")
			before := context.Line()
			if !classifyLine(context) || context.Line().LineType != model.LineTypeContent || !parseLine(context) {
				t.Fatal("Текст не разобран как content")
			}
			if len(context.Line().Elements) != 1 || len(context.Diagnostics()) != 0 {
				t.Fatalf("Содержимое интерпретировано повторно: %+v, %+v", context.Line().Elements, context.Diagnostics())
			}
			assertOperandElement(t, context.Line().Elements[0], operandWant(model.ElementTypeContent, text, text, 3))
			assertLineServiceFields(t, context, before)
		})
	}
}

func TestInvalidHeadingsRemainPartialWithoutInventedDiagnostic(t *testing.T) {
	// Ошибки формы из DSL 1.29 и 2.1.1–2.1.5 не превращаются в P003 для тега.
	for _, source := range []string{"#Title", "#", "##", "### \t", "#### Title", "##### Title", "#\tTitle", "##\u00a0Title"} {
		t.Run(source, func(t *testing.T) {
			context := newLineTestContext(t, source)
			before := context.Line()
			if !classifyLine(context) || context.Line().LineType != model.LineTypeInvalid || parseLine(context) {
				t.Fatal("Ошибочный заголовок принят за завершённый разбор")
			}
			parseHeading(context)
			if len(context.Line().Elements) != 0 || len(context.Diagnostics()) != 0 {
				t.Fatal("Нераспознанному заголовку назначены неподтверждённые элементы или диагностика")
			}
			assertLineServiceFields(t, context, before)
		})
	}
}

func TestLineBracesAndDiagnosedErrors(t *testing.T) {
	for _, test := range []struct {
		source     string
		kind       model.LineType
		elements   []model.Element
		code       diagnostics.Code
		scope      diagnostics.Scope
		start, end int
	}{
		{"{", model.LineTypeInvalid, []model.Element{{ElementType: model.ElementTypeUnparsed, Raw: "{", Start: 1, End: 2}}, diagnostics.P008, diagnostics.ScopeElement, 1, 2},
		{" \t{ \t", model.LineTypeInvalid, []model.Element{{ElementType: model.ElementTypeUnparsed, Raw: "{", Start: 3, End: 4}}, diagnostics.P008, diagnostics.ScopeElement, 3, 4},
		{"}", model.LineTypeBlockEnd, []model.Element{operandWant(model.ElementTypeBlockClose, "}", "}", 1)}, "", "", 0, 0},
		{" \t} \t", model.LineTypeBlockEnd, []model.Element{operandWant(model.ElementTypeBlockClose, "}", "}", 3)}, "", "", 0, 0},
		{"} @note Text", model.LineTypeBlockEnd, []model.Element{operandWant(model.ElementTypeBlockClose, "}", "}", 1)}, diagnostics.P010, diagnostics.ScopeLine, 1, 13},
		{"} }", model.LineTypeBlockEnd, []model.Element{operandWant(model.ElementTypeBlockClose, "}", "}", 1)}, diagnostics.P010, diagnostics.ScopeLine, 1, 4},
		{" \t}Ёж🌍 \t", model.LineTypeBlockEnd, []model.Element{operandWant(model.ElementTypeBlockClose, "}", "}", 3)}, diagnostics.P010, diagnostics.ScopeLine, 3, 7},
		{"@unknown", model.LineTypeInvalid, []model.Element{{ElementType: model.ElementTypeUnparsed, Raw: "@unknown", Start: 1, End: 9}}, diagnostics.P003, diagnostics.ScopeElement, 1, 9},
		{"@note{", model.LineTypeBlockStart, []model.Element{operandWant(model.ElementTypeTag, "@note", "note", 1), operandWant(model.ElementTypeBlockOpen, "{", "{", 6)}, diagnostics.P004, diagnostics.ScopeLine, 6, 7},
		{"# Use {x}", model.LineTypeHeading, []model.Element{operandWant(model.ElementTypeHeadingLevel, "#", "1", 1), operandWant(model.ElementTypeTitle, "Use {x}", "Use {x}", 3)}, diagnostics.P012, diagnostics.ScopeElement, 3, 10},
	} {
		t.Run(test.source, func(t *testing.T) {
			context := newLineTestContext(t, test.source)
			before := context.Line()
			if !classifyLine(context) || !parseLine(context) || context.Line().LineType != test.kind {
				t.Fatalf("Неверный тип или незавершённый разбор: %+v", context.Line())
			}
			if len(context.Line().Elements) != len(test.elements) {
				t.Fatalf("Лишняя либо потерянная конструкция: %+v", context.Line().Elements)
			}
			for i, element := range test.elements {
				assertOperandElement(t, context.Line().Elements[i], element)
			}
			got := context.Diagnostics()
			if test.code == "" {
				if len(got) != 0 {
					t.Fatalf("Закрытие не должно получать P009 на этом уровне: %+v", got)
				}
			} else {
				want := model.Location{Start: model.Position{Line: 19, Column: test.start}, End: model.Position{Line: 19, Column: test.end}}
				if len(got) != 1 || got[0].DiagnosticCode != test.code || got[0].DiagnosticScope != test.scope || got[0].Location == nil || *got[0].Location != want || got[0].ID != "" || got[0].Source != "" {
					t.Fatalf("Диагностики %+v, требуется %s/%s в %+v", got, test.code, test.scope, want)
				}
			}
			assertLineServiceFields(t, context, before)
		})
	}
}

func TestLineParsingHonoursAssignedTypeAndUnavailableInput(t *testing.T) {
	if classifyLine(nil) || parseLine(nil) {
		t.Fatal("nil-контекст принят как успешно обработанный")
	}
	parseHeading(nil)
	parseContent(nil)
	parseClosing(nil)
	for _, test := range []struct {
		source string
		kind   model.LineType
	}{
		{"", model.LineTypeContent}, {" \t", model.LineTypeContent},
		{"text", model.LineTypeHeading}, {"", model.LineTypeHeading},
		{" \t", model.LineTypeBlockEnd}, {`\}`, model.LineTypeBlockEnd},
		{"text", model.LineTypeTag}, {"text", model.LineTypeBlockStart},
		{"text", ""}, {"---", model.LineTypeSeparator}, {"text", model.LineTypeInvalid},
	} {
		t.Run(string(test.kind)+test.source, func(t *testing.T) {
			context := newLineTestContext(t, test.source)
			line := context.Line()
			line.LineType = test.kind
			context.SetLine(line)
			if parseLine(context) || len(context.Line().Elements) != 0 || len(context.Diagnostics()) != 0 {
				t.Fatal("Неустановленный или несовместимый тип принят как завершённый разбор")
			}
			assertLineServiceFields(t, context, line)
		})
	}
	// Тип content направляет к цельному тексту даже при начале, похожем на тег.
	context := newLineTestContext(t, "@note Text")
	line := context.Line()
	line.LineType = model.LineTypeContent
	context.SetLine(line)
	if !parseLine(context) || len(context.Line().Elements) != 1 {
		t.Fatal("Не учтён установленный тип content")
	}
	assertOperandElement(t, context.Line().Elements[0], operandWant(model.ElementTypeContent, "@note Text", "@note Text", 1))
}

func TestLineParsingPreservesExistingResultsAndEndings(t *testing.T) {
	for _, ending := range []model.LineEnding{model.LineEndingLF, model.LineEndingCRLF, model.LineEndingNone} {
		t.Run(string(ending), func(t *testing.T) {
			context := newLineTestContext(t, "  ## Vocabulary")
			line := context.Line()
			line.Number, line.LineEnding, line.HasErrors = 73, ending, true
			prefix := operandWant(model.ElementTypeContent, " ", " ", 1)
			prefix.ErrorIDs = []string{"previous"}
			line.Elements = []model.Element{prefix}
			context.SetLine(line)
			context.AddDiagnostic(model.Diagnostic{ID: "previous", Source: "parser", DiagnosticCode: diagnostics.P004})
			if !classifyLine(context) || len(context.Line().Elements) != 1 || !parseLine(context) {
				t.Fatal("Утрачен ранее записанный результат или разбор не завершён")
			}
			assertLineServiceFields(t, context, line)
			got, errors := context.Line().Elements, context.Diagnostics()
			if len(got) != 3 || got[0].Raw != prefix.Raw || got[0].Value != prefix.Value || len(got[0].ErrorIDs) != 1 || got[0].ErrorIDs[0] != "previous" || len(errors) != 1 || errors[0].ID != "previous" || errors[0].Source != "parser" || errors[0].DiagnosticCode != diagnostics.P004 {
				t.Fatal("Изменены ранее записанные элементы или диагностики")
			}
			assertOperandElement(t, got[1], operandWant(model.ElementTypeHeadingLevel, "##", "2", 3))
			assertOperandElement(t, got[2], operandWant(model.ElementTypeTitle, "Vocabulary", "Vocabulary", 6))
		})
	}
	context := grammar.NewContext("}")
	context.SetLine(model.Line{Number: 1, Raw: "}"})
	context.SetDiagnosticRegistry(diagnostics.NewRegistry())
	if !classifyLine(context) || !parseLine(context) || len(context.Diagnostics()) != 0 || context.Line().ParentLine != nil || context.Line().NestingLevel != 0 {
		t.Fatal("Закрытие без родителя получило P009 или изменило вложенность")
	}
	assertOperandElement(t, context.Line().Elements[0], operandWant(model.ElementTypeBlockClose, "}", "}", 1))
}

func newLineTestContext(t *testing.T, source string) grammar.GrammarContext {
	t.Helper()
	context := grammar.NewContext(source)
	context.SetDiagnosticRegistry(diagnostics.NewRegistry())
	parent := 4
	context.SetLine(model.Line{Number: 19, Raw: source, LineEnding: model.LineEndingCRLF, ParentLine: &parent, NestingLevel: 6})
	context.AddDetection("preserved", 17)
	return context
}

func assertLineServiceFields(t *testing.T, context grammar.GrammarContext, before model.Line) {
	t.Helper()
	got := context.Line()
	if got.Number != before.Number || got.Raw != before.Raw || got.LineEnding != before.LineEnding || got.ParentLine != before.ParentLine || *got.ParentLine != 4 || got.NestingLevel != before.NestingLevel || got.HasErrors != before.HasErrors || context.String() != before.Raw {
		t.Fatalf("Изменены служебные данные строки: %+v, было %+v", got, before)
	}
	if count, found := context.Detection("preserved"); !found || count != 17 {
		t.Fatal("Изменены ранее рассчитанные признаки")
	}
	for _, element := range got.Elements {
		if element.Start < 1 || element.End > utf8.RuneCountInString(context.String())+1 || element.End < element.Start || string([]rune(context.String())[element.Start-1:element.End-1]) != element.Raw {
			t.Errorf("Нарушен исходный Unicode-диапазон: %+v", element)
		}
	}
	if strings.Contains(context.String(), "}") {
		for _, diagnostic := range context.Diagnostics() {
			if diagnostic.DiagnosticCode == diagnostics.P009 {
				t.Fatal("Локальный разбор проверил существование открытого блока")
			}
		}
	}
}
