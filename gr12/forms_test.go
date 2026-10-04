package gr12

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestFormsFromDSLExamples(t *testing.T) {
	for _, test := range []struct {
		tag, source, value string
		kind               model.ElementType
		block              bool
	}{
		// DSL 1.2, 1.32–1.34, 2.2–2.8: самостоятельные теги и редакторский текст.
		{"dsl-version", "@dsl-version 1.2", "1.2", model.ElementTypeVersion, false},
		{"document-id", "@document-id scenario-7f3a91c2", "scenario-7f3a91c2", model.ElementTypeIdentifier, false},
		{"order", "@order 20", "20", model.ElementTypeNumber, false},
		{"section", "@section Studentbook", "Studentbook", model.ElementTypeName, false},
		{"header", "@header Vocabulary practice", "Vocabulary practice", model.ElementTypeTitle, false},
		{"task", "@task task-a", "task-a", model.ElementTypeIdentifier, false},
		{"endtask", "@endtask", "", "", false},
		{"step", "@step Step 2. Read the text", "Step 2. Read the text", model.ElementTypeTitle, false},
		{"speaking", "@speaking", "", "", false},
		{"newpage", "@newpage", "", "", false},
		{"editor", "@editor Check the wording before publication.", "Check the wording before publication.", model.ElementTypeContent, false},
		{"editor", "@editor {", "", "", true},
		// DSL 3.2–3.13: текст, название, обязательный блок и ID фрагмента.
		{"example", "@example He live in London - He lives in London.", "He live in London - He lives in London.", model.ElementTypeContent, false},
		{"example", "@example How to {", "How to", model.ElementTypeTitle, true},
		{"wordlist", "@wordlist Italian; Spanish; British.", "Italian; Spanish; British.", model.ElementTypeContent, false},
		{"wordlist", "@wordlist Nationalities {", "Nationalities", model.ElementTypeTitle, true},
		{"table", "@table Pronoun forms {", "Pronoun forms", model.ElementTypeTitle, true},
		{"script", "@script Track 01 — Transcript {", "Track 01 — Transcript", model.ElementTypeTitle, true},
		{"text", "@text City Life {", "City Life", model.ElementTypeTitle, true},
		{"key", "@key Answers {", "Answers", model.ElementTypeTitle, true},
		{"instr", "@instr Read the text and answer the questions.", "Read the text and answer the questions.", model.ElementTypeContent, false},
		{"instr", "@instr {", "", "", true},
		{"note", "@note Remember to use capital letters.", "Remember to use capital letters.", model.ElementTypeContent, false},
		{"note", "@note Tip {", "Tip", model.ElementTypeTitle, true},
		{"alt", "@alt Улыбающаяся семья сидит на диване.", "Улыбающаяся семья сидит на диване.", model.ElementTypeContent, false},
		{"alt", "@alt Изображение 4A {", "Изображение 4A", model.ElementTypeTitle, true},
		{"hint", "@hint Remember to use the third-person singular form.", "Remember to use the third-person singular form.", model.ElementTypeContent, false},
		{"hint", "@hint {", "", "", true},
		{"fragment", "@fragment grammar-note {", "grammar-note", model.ElementTypeIdentifier, true},
		{"include", "@include grammar-note", "grammar-note", model.ElementTypeIdentifier, false},
		// DSL 4.1–4.9: инструкции имеют тип content, включая все четыре multifill.
		{"answer", "@answer New York", "New York", model.ElementTypeContent, false},
		{"answer", "@answer {", "", "", true},
		{"question", "@question What is your name?", "What is your name?", model.ElementTypeContent, false},
		{"multifill", "@multifill", "", "", false},
		{"multifill", "@multifill Write your answer.", "Write your answer.", model.ElementTypeContent, false},
		{"multifill", "@multifill {", "", "", true},
		{"multifill", "@multifill Complete the text. {", "Complete the text.", model.ElementTypeContent, true},
		{"choice", "@choice Choose a country. {", "Choose a country.", model.ElementTypeContent, true},
		{"multichoice", "@multichoice Select your favourite activities. {", "Select your favourite activities.", model.ElementTypeContent, true},
		{"matching", "@matching Answer each question. {", "Answer each question.", model.ElementTypeContent, true},
		{"ordering", "@ordering Put the stages in order. {", "Put the stages in order.", model.ElementTypeContent, true},
		{"variants", "@variants {", "", "", true},
		{"variant", "@variant Student A", "Student A", model.ElementTypeName, false},
	} {
		t.Run(test.source, func(t *testing.T) {
			var parts []formPart
			if test.value != "" {
				parts = append(parts, formPart{test.kind, test.value, len(test.tag) + 3})
			}
			if test.block {
				parts = append(parts, formPart{model.ElementTypeBlockOpen, "{", utf8.RuneCountInString(test.source)})
			}
			assertForm(t, test.tag, test.source, test.block, parts, nil)
		})
	}
}

func TestBlockFamiliesWithoutOptionalTextAndWithOpeningTail(t *testing.T) {
	for _, tag := range []string{"editor", "instr", "hint", "answer", "example", "wordlist", "note", "alt", "table", "script", "text", "key", "choice", "multichoice", "matching", "ordering", "variants", "multifill"} {
		t.Run(tag, func(t *testing.T) {
			brace := len(tag) + 3
			parts := []formPart{{model.ElementTypeBlockOpen, "{", brace}}
			assertForm(t, tag, "@"+tag+" {", true, parts, nil)
			assertForm(t, tag, "@"+tag+" { \t", true, parts, nil)
			assertForm(t, tag, "@"+tag+" { хвост", true, parts, []formDiagnostic{{diagnostics.P008, brace, brace + 7}})
			assertForm(t, tag, "@"+tag+" { }", true, parts, []formDiagnostic{{diagnostics.P008, brace, brace + 3}})
			assertForm(t, tag, "@"+tag+"{", true, []formPart{{model.ElementTypeBlockOpen, "{", brace - 1}}, []formDiagnostic{{diagnostics.P004, brace - 1, brace}})
		})
	}
}

func TestFormRegressionsPreserveStructureAndNeighbours(t *testing.T) {
	for _, test := range []struct {
		tag, source string
		block       bool
		parts       []formPart
		diagnostics []formDiagnostic
	}{
		{"instr", "@instr underline the text { }", false, []formPart{{model.ElementTypeContent, "underline the text { }", 8}}, []formDiagnostic{{diagnostics.P012, 8, 30}}},
		{"hint", "@hint Title {", true, []formPart{{model.ElementTypeBlockOpen, "{", 13}}, []formDiagnostic{{diagnostics.P007, 7, 12}}},
		{"instr", "@instr Title {", true, []formPart{{model.ElementTypeBlockOpen, "{", 14}}, []formDiagnostic{{diagnostics.P007, 8, 13}}},
		{"fragment", "@fragment part extra {", true, []formPart{{model.ElementTypeIdentifier, "part", 11}, {model.ElementTypeBlockOpen, "{", 22}}, []formDiagnostic{{diagnostics.P007, 16, 21}}},
		{"variants", "@variants Title {", true, []formPart{{model.ElementTypeBlockOpen, "{", 17}}, []formDiagnostic{{diagnostics.P007, 11, 16}}},
		{"task", "@task task-1 {", false, []formPart{{model.ElementTypeIdentifier, "task-1", 7}}, []formDiagnostic{{diagnostics.P005, 1, 15}}},
		{"question", "@question {", false, nil, []formDiagnostic{{diagnostics.P005, 1, 12}}},
		{"variant", "@variant A {", false, []formPart{{model.ElementTypeName, "A", 10}}, []formDiagnostic{{diagnostics.P005, 1, 13}}},
		{"table", "@table Data", false, []formPart{{model.ElementTypeTitle, "Data", 8}}, []formDiagnostic{{diagnostics.P005, 1, 12}}},
		{"choice", "@choice Choose one.", false, []formPart{{model.ElementTypeContent, "Choose one.", 9}}, []formDiagnostic{{diagnostics.P005, 1, 20}}},
		{"fragment", "@fragment part", false, []formPart{{model.ElementTypeIdentifier, "part", 11}}, []formDiagnostic{{diagnostics.P005, 1, 15}}},
		{"variants", "@variants", false, nil, []formDiagnostic{{diagnostics.P005, 1, 10}}},
		{"fragment", "@fragment {", true, []formPart{{model.ElementTypeBlockOpen, "{", 11}}, []formDiagnostic{{diagnostics.P006, 11, 11}}},
		{"fragment", "@fragment", false, nil, []formDiagnostic{{diagnostics.P006, 10, 10}, {diagnostics.P005, 1, 10}}},
		{"fragment", "@fragment Part{tail", true, []formPart{{model.ElementTypeIdentifier, "Part", 11}, {model.ElementTypeBlockOpen, "{", 15}}, []formDiagnostic{{diagnostics.P004, 15, 16}, {diagnostics.P008, 15, 20}}},
		{"fragment", "@fragment part extra { x", true, []formPart{{model.ElementTypeIdentifier, "part", 11}, {model.ElementTypeBlockOpen, "{", 22}}, []formDiagnostic{{diagnostics.P007, 16, 21}, {diagnostics.P008, 22, 25}}},
		{"endtask", "@endtask task-1", false, nil, []formDiagnostic{{diagnostics.P007, 10, 16}}},
		{"endtask", "@endtask {", false, nil, []formDiagnostic{{diagnostics.P005, 1, 11}}},
		{"include", "@include part extra", false, []formPart{{model.ElementTypeIdentifier, "part", 10}}, []formDiagnostic{{diagnostics.P007, 15, 20}}},
		{"task", "@task", false, nil, []formDiagnostic{{diagnostics.P006, 6, 6}}},
		// Уточнение пользователя: первая неэкранированная { открывает эти семейства.
		{"note", "@note Text {x}", true, []formPart{{model.ElementTypeTitle, "Text", 7}, {model.ElementTypeBlockOpen, "{", 12}}, []formDiagnostic{{diagnostics.P008, 12, 15}}},
		{"multifill", "@multifill Text {x}", true, []formPart{{model.ElementTypeContent, "Text", 12}, {model.ElementTypeBlockOpen, "{", 17}}, []formDiagnostic{{diagnostics.P008, 17, 20}}},
		{"text", "@text City Life { Hello", true, []formPart{{model.ElementTypeTitle, "City Life", 7}, {model.ElementTypeBlockOpen, "{", 17}}, []formDiagnostic{{diagnostics.P008, 17, 24}}},
		{"note", "@note Title{", true, []formPart{{model.ElementTypeTitle, "Title", 7}, {model.ElementTypeBlockOpen, "{", 12}}, []formDiagnostic{{diagnostics.P004, 12, 13}}},
		{"note", `@note "{x}"`, true, []formPart{{model.ElementTypeTitle, `"`, 7}, {model.ElementTypeBlockOpen, "{", 8}}, []formDiagnostic{{diagnostics.P004, 8, 9}, {diagnostics.P008, 8, 12}}},
		{"instr", `@instr "{x}"`, false, []formPart{{model.ElementTypeContent, `"{x}"`, 8}}, []formDiagnostic{{diagnostics.P012, 8, 13}}},
		{"question", "@question Use {x} here.", false, []formPart{{model.ElementTypeContent, "Use {x} here.", 11}}, []formDiagnostic{{diagnostics.P012, 11, 24}}},
		{"variant", " \t@VaRiAnT A { \t", false, []formPart{{model.ElementTypeName, "A", 12}}, []formDiagnostic{{diagnostics.P005, 3, 15}}},
	} {
		t.Run(test.source, func(t *testing.T) {
			assertForm(t, test.tag, test.source, test.block, test.parts, test.diagnostics)
		})
	}
}

func TestFormValuesKeepUnicodeWhitespaceAndEscaping(t *testing.T) {
	for _, test := range []struct {
		tag, source, raw string
		kind             model.ElementType
		start            int
	}{
		{"order", "@order 0020", "0020", model.ElementTypeNumber, 8},
		{"dsl-version", "@dsl-version 9.9", "9.9", model.ElementTypeVersion, 14},
		{"task", "@task Задача_Ёж🌍", "Задача_Ёж🌍", model.ElementTypeIdentifier, 7},
		{"document-id", "@document-id MiXeD_ID", "MiXeD_ID", model.ElementTypeIdentifier, 14},
		{"section", " \t@section Student\tBook \t", "Student\tBook", model.ElementTypeName, 12},
		{"step", "@step Этап 🌍\tА", "Этап 🌍\tА", model.ElementTypeTitle, 7},
		{"wordlist", "@wordlist Italian;\tSpanish; British.", "Italian;\tSpanish; British.", model.ElementTypeContent, 11},
		{"instr", `@instr Use \{ and \} as text.`, `Use \{ and \} as text.`, model.ElementTypeContent, 8},
		{"instr", " \t@INSTR  Ёж🌍\tА \t", "Ёж🌍\tА", model.ElementTypeContent, 11},
		{"note", `@note Use \{ and \} as text.`, `Use \{ and \} as text.`, model.ElementTypeContent, 7},
		{"multifill", `@multifill Use \{ and \} as text.`, `Use \{ and \} as text.`, model.ElementTypeContent, 12},
	} {
		t.Run(test.source, func(t *testing.T) {
			assertForm(t, test.tag, test.source, false, []formPart{{test.kind, test.raw, test.start}}, nil)
		})
	}
}

func TestFormBoundariesAndEscapeParity(t *testing.T) {
	for _, family := range []struct {
		tag  string
		kind model.ElementType
	}{
		{"note", model.ElementTypeTitle}, {"text", model.ElementTypeTitle},
		{"choice", model.ElementTypeContent}, {"multifill", model.ElementTypeContent},
	} {
		t.Run(family.tag, func(t *testing.T) {
			value := "Ёж🌍\t\\{ \\}"
			start := len(family.tag) + 4
			brace := start + utf8.RuneCountInString(value) + 1
			assertForm(t, family.tag, "\t@"+strings.ToUpper(family.tag)+" "+value+" {tail \t", true,
				[]formPart{{family.kind, value, start}, {model.ElementTypeBlockOpen, "{", brace}},
				[]formDiagnostic{{diagnostics.P008, brace, brace + 5}})
			start = len(family.tag) + 3
			brace = start + 6
			assertForm(t, family.tag, "@"+family.tag+"\tTitle\t{", true,
				[]formPart{{family.kind, "Title", start}, {model.ElementTypeBlockOpen, "{", brace}},
				[]formDiagnostic{{diagnostics.P004, start - 1, start}, {diagnostics.P004, brace - 1, brace}})
			for count := 1; count <= 4; count++ {
				prefix := "Text " + strings.Repeat(`\`, count)
				source := "@" + family.tag + " " + prefix + "{"
				if count%2 == 0 {
					brace = start + len(prefix)
					assertForm(t, family.tag, source, true,
						[]formPart{{family.kind, prefix, start}, {model.ElementTypeBlockOpen, "{", brace}},
						[]formDiagnostic{{diagnostics.P004, brace, brace + 1}})
				} else {
					kind := model.ElementTypeContent
					var errors []formDiagnostic
					if family.tag == "text" || family.tag == "choice" {
						kind = family.kind
						errors = []formDiagnostic{{diagnostics.P005, 1, utf8.RuneCountInString(source) + 1}}
					}
					assertForm(t, family.tag, source, false, []formPart{{kind, prefix + "{", start}}, errors)
				}
			}
		})
	}
	assertForm(t, "fragment", "@fragment\tPart\t{", true,
		[]formPart{{model.ElementTypeIdentifier, "Part", 11}, {model.ElementTypeBlockOpen, "{", 16}},
		[]formDiagnostic{{diagnostics.P004, 10, 11}, {diagnostics.P004, 15, 16}})
	assertForm(t, "instr", "@instr Title{", true, []formPart{{model.ElementTypeBlockOpen, "{", 13}},
		[]formDiagnostic{{diagnostics.P007, 8, 13}, {diagnostics.P004, 13, 14}})
	assertForm(t, "variant", "@variant\tЁж🌍\tA", false, []formPart{{model.ElementTypeName, "Ёж🌍\tA", 10}},
		[]formDiagnostic{{diagnostics.P004, 9, 10}})
	assertForm(t, "task", "@task\tMiXeD", false, []formPart{{model.ElementTypeIdentifier, "MiXeD", 7}},
		[]formDiagnostic{{diagnostics.P004, 6, 7}})
}

func TestFormFamiliesWithoutArgumentsOrPreparedTag(t *testing.T) {
	for _, family := range []struct {
		tags  string
		codes []diagnostics.Code
	}{
		{"endtask speaking newpage section header step question variant editor instr hint answer example wordlist note alt multifill", nil},
		{"dsl-version document-id order task include", []diagnostics.Code{diagnostics.P006}},
		{"table script text key choice multichoice matching ordering variants", []diagnostics.Code{diagnostics.P005}},
		{"fragment", []diagnostics.Code{diagnostics.P006, diagnostics.P005}},
	} {
		for _, tag := range strings.Fields(family.tags) {
			t.Run(tag, func(t *testing.T) {
				var errors []formDiagnostic
				for _, code := range family.codes {
					start, end := 1, len(tag)+2
					if code == diagnostics.P006 {
						start = end
					}
					errors = append(errors, formDiagnostic{code, start, end})
				}
				assertForm(t, tag, "@"+tag, false, nil, errors)
				parseFormForTest(t, nil, tag)
				context := grammar.NewContext("@" + tag)
				parseFormForTest(t, context, tag)
				if len(context.Line().Elements) != 0 || len(context.Diagnostics()) != 0 || context.Line().LineType != "" {
					t.Fatal("Разбор без подготовленного тега изменил контекст")
				}
			})
		}
	}
}

func TestUnsupportedFormsKeepSeparatorDiagnostics(t *testing.T) {
	for _, tag := range []string{"endtask", "speaking", "newpage", "dsl-version", "document-id", "order", "task", "include"} {
		operands := []string{""}
		switch tag {
		case "dsl-version", "document-id", "order", "task", "include":
			operands = []string{"", " value"}
		}
		for _, operand := range operands {
			for _, separator := range []string{"", "\t", " "} {
				source := "@" + tag + operand + separator + "{"
				t.Run(source, func(t *testing.T) {
					context := newOperandContext(t, source, "")
					if !classifyLine(context) || !parseLine(context) {
						t.Fatal("Не завершён разбор ошибочной формы")
					}
					wantCodes := []diagnostics.Code{}
					if len(operands) == 2 && operand == "" {
						wantCodes = append(wantCodes, diagnostics.P006)
					}
					if separator != " " {
						wantCodes = append(wantCodes, diagnostics.P004)
					}
					wantCodes = append(wantCodes, diagnostics.P005)
					got := context.Diagnostics()
					if len(got) != len(wantCodes) {
						t.Fatalf("Диагностики %+v, требуются %v", got, wantCodes)
					}
					for i, code := range wantCodes {
						if got[i].DiagnosticCode != code {
							t.Fatalf("Диагностика %d: %+v, требуется %s", i, got[i], code)
						}
						if code == diagnostics.P004 && (got[i].Location == nil || got[i].Location.Start.Column != len(tag)+len(operand)+2) {
							t.Fatalf("Неверный диапазон P004: %+v", got[i])
						}
					}
					wantElements := 1
					if operand != "" {
						wantElements++
					}
					if context.Line().LineType != model.LineTypeTag || len(context.Line().Elements) != wantElements || increasesNesting(grammar.AssertionInput{Line: context.Line()}) {
						t.Fatal("Запрещённая форма изменила элементы или открыла уровень")
					}
				})
			}
		}
	}
}

// formPart описывает ожидаемый фрагмент; значение в этих формах совпадает с raw.
type formPart struct {
	kind  model.ElementType
	raw   string
	start int
}

// formDiagnostic задаёт ожидаемый код и исключающий правый край диапазона.
type formDiagnostic struct {
	code       diagnostics.Code
	start, end int
}

func assertForm(t *testing.T, tag, source string, block bool, parts []formPart, expectedDiagnostics []formDiagnostic) {
	t.Helper()
	context := newOperandContext(t, source, tag)
	line := context.Line()
	parent := 3
	line.ParentLine, line.NestingLevel = &parent, 2
	context.SetLine(line)
	tagElement := line.Elements[0]
	parseFormForTest(t, context, tag)
	got := context.Line()
	wantType := model.LineTypeTag
	if block {
		wantType = model.LineTypeBlockStart
	}
	if got.LineType != wantType || got.Raw != source || context.String() != source || got.Number != 19 || got.NestingLevel != 2 || got.ParentLine != &parent || parent != 3 || got.HasErrors {
		t.Fatalf("Неверные свойства строки: %+v, требуется тип %s", got, wantType)
	}
	if len(got.Elements) != len(parts)+1 {
		t.Fatalf("Элементы %+v, требуется тег и %d частей", got.Elements, len(parts))
	}
	assertOperandElement(t, got.Elements[0], tagElement)
	for i, part := range parts {
		assertOperandElement(t, got.Elements[i+1], operandWant(part.kind, part.raw, part.raw, part.start))
	}
	actualDiagnostics := context.Diagnostics()
	if len(actualDiagnostics) != len(expectedDiagnostics) {
		t.Fatalf("Диагностики %+v, требуются %+v", actualDiagnostics, expectedDiagnostics)
	}
	for i, expected := range expectedDiagnostics {
		d := actualDiagnostics[i]
		description, _ := context.Lookup(expected.code)
		location := model.Location{Start: model.Position{Line: 19, Column: expected.start}, End: model.Position{Line: 19, Column: expected.end}}
		if d.DiagnosticCode != expected.code || d.DiagnosticScope != description.Scope() || d.Location == nil || *d.Location != location || d.ID != "" || d.Source != "" {
			t.Errorf("Диагностика %+v в %+v, требуется %+v", d, d.Location, expected)
		}
	}
}

// parseFormForTest вызывает только заданное семейство; имя уже подготовлено в модели.
func parseFormForTest(t *testing.T, context grammar.GrammarContext, tag string) {
	t.Helper()
	switch strings.ToLower(tag) {
	case "endtask", "speaking", "newpage":
		parseBareTag(context)
	case "dsl-version":
		parseTokenTag(context, model.ElementTypeVersion)
	case "document-id", "task", "include":
		parseTokenTag(context, model.ElementTypeIdentifier)
	case "order":
		parseTokenTag(context, model.ElementTypeNumber)
	case "section", "variant":
		parseFreeTag(context, model.ElementTypeName)
	case "header", "step":
		parseFreeTag(context, model.ElementTypeTitle)
	case "question":
		parseFreeTag(context, model.ElementTypeContent)
	case "editor", "instr", "hint", "answer":
		parsePlainTextOrBlock(context)
	case "example", "wordlist", "note", "alt":
		parseTitledTextOrBlock(context)
	case "table", "script", "text", "key":
		parseTitledBlock(context)
	case "choice", "multichoice", "matching", "ordering":
		parseInstructionBlock(context)
	case "fragment":
		parseIDBlock(context)
	case "variants":
		parseBareBlock(context)
	case "multifill":
		parseMultifill(context)
	default:
		t.Fatalf("Неизвестное тестовое семейство %s", tag)
	}
}
