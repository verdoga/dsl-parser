package gr12

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
	"github.com/verdoga/dsl-parser/parser"
)

func TestOrchestrateLineCompletesLocalParsingOnce(t *testing.T) {
	for _, test := range []struct {
		source   string
		kind     model.LineType
		elements []model.Element
		code     diagnostics.Code
		scope    diagnostics.Scope
		start    int
		end      int
	}{
		{source: "", kind: model.LineTypeBlank},
		{source: " \t \t", kind: model.LineTypeBlank},
		// DSL 1.2 и 2.1: технический тег и синтаксические уровни заголовков.
		{source: "@dsl-version 1.2", kind: model.LineTypeTag, elements: []model.Element{
			operandWant(model.ElementTypeTag, "@dsl-version", "dsl-version", 1),
			operandWant(model.ElementTypeVersion, "1.2", "1.2", 14),
		}},
		{source: "# Unit 1", kind: model.LineTypeHeading, elements: []model.Element{
			operandWant(model.ElementTypeHeadingLevel, "#", "1", 1),
			operandWant(model.ElementTypeTitle, "Unit 1", "Unit 1", 3),
		}},
		{source: "## Vocabulary", kind: model.LineTypeHeading, elements: []model.Element{
			operandWant(model.ElementTypeHeadingLevel, "##", "2", 1),
			operandWant(model.ElementTypeTitle, "Vocabulary", "Vocabulary", 4),
		}},
		{source: " \t### Ёж🌍 \t", kind: model.LineTypeHeading, elements: []model.Element{
			operandWant(model.ElementTypeHeadingLevel, "###", "3", 3),
			operandWant(model.ElementTypeTitle, "Ёж🌍", "Ёж🌍", 7),
		}},
		{source: "Introductory text.", kind: model.LineTypeContent, elements: []model.Element{
			operandWant(model.ElementTypeContent, "Introductory text.", "Introductory text.", 1),
		}},
		// DSL 1.21, 4.3: признаки внутри текста не запускают разбор блоков и тегов.
		{source: `\@note This is ordinary text.`, kind: model.LineTypeContent, elements: []model.Element{
			operandWant(model.ElementTypeContent, `\@note This is ordinary text.`, `\@note This is ordinary text.`, 1),
		}},
		{source: "She _____{lives} in London.", kind: model.LineTypeContent, elements: []model.Element{
			operandWant(model.ElementTypeContent, "She _____{lives} in London.", "She _____{lives} in London.", 1),
		}},
		{source: ` \t`, kind: model.LineTypeContent, elements: []model.Element{
			operandWant(model.ElementTypeContent, `\t`, `\t`, 2),
		}},
		{source: "@note Text @instr More", kind: model.LineTypeTag, elements: []model.Element{
			operandWant(model.ElementTypeTag, "@note", "note", 1),
			operandWant(model.ElementTypeContent, "Text @instr More", "Text @instr More", 7),
		}},
		{source: "@instr {", kind: model.LineTypeBlockStart, elements: []model.Element{
			operandWant(model.ElementTypeTag, "@instr", "instr", 1),
			operandWant(model.ElementTypeBlockOpen, "{", "{", 8),
		}},
		{source: "@note Tip {", kind: model.LineTypeBlockStart, elements: []model.Element{
			operandWant(model.ElementTypeTag, "@note", "note", 1),
			operandWant(model.ElementTypeTitle, "Tip", "Tip", 7),
			operandWant(model.ElementTypeBlockOpen, "{", "{", 11),
		}},
		{source: " \t} \t", kind: model.LineTypeBlockEnd, elements: []model.Element{
			operandWant(model.ElementTypeBlockClose, "}", "}", 3),
		}},
		// Некорректные варианты DSL 1.18, 1.19, 1.29 остаются завершёнными.
		{source: "@noteText", kind: model.LineTypeInvalid, elements: []model.Element{
			{ElementType: model.ElementTypeUnparsed, Raw: "@noteText", Start: 1, End: 10},
		}, code: diagnostics.P003, scope: diagnostics.ScopeElement, start: 1, end: 10},
		{source: " \t{ \t", kind: model.LineTypeInvalid, elements: []model.Element{
			{ElementType: model.ElementTypeUnparsed, Raw: "{", Start: 3, End: 4},
		}, code: diagnostics.P008, scope: diagnostics.ScopeElement, start: 3, end: 4},
		{source: "@note Tip {x}", kind: model.LineTypeBlockStart, elements: []model.Element{
			operandWant(model.ElementTypeTag, "@note", "note", 1),
			operandWant(model.ElementTypeTitle, "Tip", "Tip", 7),
			operandWant(model.ElementTypeBlockOpen, "{", "{", 11),
		}, code: diagnostics.P008, scope: diagnostics.ScopeLine, start: 11, end: 14},
		{source: "} @note Text", kind: model.LineTypeBlockEnd, elements: []model.Element{
			operandWant(model.ElementTypeBlockClose, "}", "}", 1),
		}, code: diagnostics.P010, scope: diagnostics.ScopeLine, start: 1, end: 13},
	} {
		t.Run(test.source, func(t *testing.T) {
			context := newLineTestContext(t, test.source)
			if !detectLine(context) {
				t.Fatal("Не удалось подготовить признаки")
			}
			before := context.Line()
			if !orchestrateLine(context) {
				t.Fatal("Оркестрация не завершила обработку")
			}
			line := context.Line()
			if line.LineType != test.kind || len(line.Elements) != len(test.elements) {
				t.Fatalf("Неверный тип или состав элементов: %+v", line)
			}
			for i, want := range test.elements {
				assertOperandElement(t, line.Elements[i], want)
			}
			assertLineServiceFields(t, context, before)
			got := context.Diagnostics()
			if test.code == "" {
				if len(got) != 0 {
					t.Fatalf("Лишние диагностики: %+v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("Ожидалась ровно одна диагностика %s: %+v", test.code, got)
			}
			diagnostic := got[0]
			wantLocation := model.Location{
				Start: model.Position{Line: 19, Column: test.start},
				End:   model.Position{Line: 19, Column: test.end},
			}
			if diagnostic.DiagnosticCode != test.code || diagnostic.DiagnosticScope != test.scope || diagnostic.Location == nil || *diagnostic.Location != wantLocation || diagnostic.ID != "" || diagnostic.Source != "" {
				t.Fatalf("Неверная диагностика: %+v", diagnostic)
			}
		})
	}
}

func TestRegisteredParsingCompletesInvalidHeadings(t *testing.T) {
	registry := grammar.NewRegistry()
	BuiltinGr12(registry)
	registered, _ := registry.Lookup("1.2")
	for _, source := range []string{"#Title", "#### Title", "###\tTitle", "###", "###x Title", " \t##### Ёж🌍 \t"} {
		t.Run(source, func(t *testing.T) {
			for _, orchestrated := range []bool{true, false} {
				context := newLineTestContext(t, source)
				before := context.Line()
				for _, detect := range registered.DetectionFuncs() {
					if !detect(context) {
						t.Fatal("Не завершено детектирование")
					}
				}
				var ok bool
				if orchestrated {
					ok = registered.OrchestrationFuncs()[0](context)
				} else {
					ok = registered.LineTypeFuncs()[0](context) && registered.LineParserFuncs()[0](context)
				}
				if !ok || context.Line().LineType != model.LineTypeInvalid || len(context.Line().Elements) != 1 {
					t.Fatalf("Оркестрация=%t: требуется завершённый invalid с одним unparsed: %+v", orchestrated, context.Line())
				}
				raw := strings.Trim(source, " \t")
				start := utf8.RuneCountInString(source) - utf8.RuneCountInString(strings.TrimLeft(source, " \t")) + 1
				end := start + utf8.RuneCountInString(raw)
				assertOperandElement(t, context.Line().Elements[0], model.Element{ElementType: model.ElementTypeUnparsed, Raw: raw, Value: nil, Start: start, End: end, ErrorIDs: []string{}})
				location := model.Location{Start: model.Position{Line: 19, Column: start}, End: model.Position{Line: 19, Column: end}}
				got := context.Diagnostics()
				if len(got) != 1 || got[0].DiagnosticCode != diagnostics.P003 || got[0].DiagnosticScope != diagnostics.ScopeElement || got[0].Location == nil || *got[0].Location != location || got[0].Fatal || got[0].SeverityLevel != diagnostics.SeverityError {
					t.Fatalf("Требуется только P003 на диапазоне unparsed, без P015: %+v", got)
				}
				assertLineServiceFields(t, context, before)
			}
		})
	}
}

func TestOrchestrateLineReportsIncompleteProcessing(t *testing.T) {
	if orchestrateLine(nil) {
		t.Fatal("Отсутствующий контекст принят")
	}
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "no detections", true: "partial detections"}[partial], func(t *testing.T) {
			context := newLineTestContext(t, "@note Tip {")
			if partial {
				context.AddDetection(spacesKind, 2)
			}
			before := syntaxLineSnapshot(t, context)
			if orchestrateLine(context) || syntaxLineSnapshot(t, context) != before || len(context.Diagnostics()) != 0 {
				t.Fatal("Обработка начата до завершения детектирования")
			}
		})
	}
}

func TestOrchestrateLineKeepsSequentialContextsIndependent(t *testing.T) {
	var contexts []grammar.GrammarContext
	var snapshots []string
	// Номер и родитель одинаковы: локальная оркестрация не использует их как состояние.
	for _, source := range []string{"}", "@note Tip {x}", "@instr {", "@noteText", "", "Introductory text.", "}", "@instr {"} {
		context := newLineTestContext(t, source)
		detectLine(context)
		before := context.Line()
		if !orchestrateLine(context) {
			t.Fatalf("Не обработана строка %q", source)
		}
		assertLineServiceFields(t, context, before)
		contexts = append(contexts, context)
		for i, previous := range contexts {
			snapshot, err := json.Marshal(struct {
				Line        model.Line
				Diagnostics []model.Diagnostic
			}{previous.Line(), previous.Diagnostics()})
			if err != nil {
				t.Fatal(err)
			}
			if i == len(snapshots) {
				snapshots = append(snapshots, string(snapshot))
			} else if string(snapshot) != snapshots[i] {
				t.Fatalf("Изменён предыдущий контекст %d", i)
			}
		}
	}
	if snapshots[0] != snapshots[6] || snapshots[2] != snapshots[7] {
		t.Fatal("Результат строки зависит от предыдущих вызовов")
	}
}

func TestRegisteredParsingReportsIncompleteOperands(t *testing.T) {
	registry := grammar.NewRegistry()
	BuiltinGr12(registry)
	registered, _ := registry.Lookup("1.2")
	media := []model.Element{operandWant(model.ElementTypeTag, "@media", "media", 1), operandWant(model.ElementTypeMediaType, "audio", "audio", 8)}
	invalidMedia := []model.Element{media[0], {ElementType: model.ElementTypeUnparsed, Raw: "vidio", Start: 8, End: 13}}
	resource := []model.Element{operandWant(model.ElementTypeTag, "@resource-dir", "resource-dir", 1)}
	for _, test := range []struct {
		source      string
		ok          bool
		elements    []model.Element
		diagnostics []formDiagnostic
	}{
		{`@media audio "unclosed`, false, media, nil},
		{`@media audio "a\"`, false, media, nil},
		{`@media audio "Ёж🌍`, false, media, nil},
		{"@media\taudio \"unclosed", false, media, []formDiagnostic{{diagnostics.P004, 7, 8}}},
		{`@media audio "a\"b"`, true, append(append([]model.Element{}, media...), operandWant(model.ElementTypeSource, `"a\"b"`, `a\"b`, 14)), nil},
		{"@media audio", true, append(append([]model.Element{}, media...), model.Element{ElementType: model.ElementTypeUnparsed, Start: 13, End: 13}), []formDiagnostic{{diagnostics.P006, 13, 13}}},
		{"@media", true, []model.Element{media[0], {ElementType: model.ElementTypeUnparsed, Start: 7, End: 7}}, []formDiagnostic{{diagnostics.P006, 7, 7}}},
		{"@media \t", true, []model.Element{media[0], {ElementType: model.ElementTypeUnparsed, Start: 9, End: 9}}, []formDiagnostic{{diagnostics.P006, 9, 9}}},
		{"@media vidio", true, append(append([]model.Element{}, invalidMedia...), model.Element{ElementType: model.ElementTypeUnparsed, Start: 13, End: 13}), []formDiagnostic{{diagnostics.P006, 8, 13}, {diagnostics.P006, 13, 13}}},
		{`@media vidio "Unit 07"`, true, append(append([]model.Element{}, invalidMedia...), operandWant(model.ElementTypeSource, `"Unit 07"`, "Unit 07", 14)), []formDiagnostic{{diagnostics.P006, 8, 13}}},
		{`@media vidio "unclosed`, false, invalidMedia, []formDiagnostic{{diagnostics.P006, 8, 13}}},
		{`@media vidio "Ёж🌍`, false, invalidMedia, []formDiagnostic{{diagnostics.P006, 8, 13}}},
		{`@media vidio "a\"`, false, invalidMedia, []formDiagnostic{{diagnostics.P006, 8, 13}}},
		{"@media\tvidio\t\"unclosed", false, invalidMedia, []formDiagnostic{{diagnostics.P004, 7, 8}, {diagnostics.P006, 8, 13}, {diagnostics.P004, 13, 14}}},
		{`@resource-dir "unclosed`, false, resource, nil},
		{`@resource-dir Audio, "unclosed`, false, append(append([]model.Element{}, resource...), operandWant(model.ElementTypeResourcePath, "Audio", "Audio", 15)), nil},
		{`@resource-dir Ёж🌍, "unclosed`, false, append(append([]model.Element{}, resource...), operandWant(model.ElementTypeResourcePath, "Ёж🌍", "Ёж🌍", 15)), nil},
		{`@resource-dir "Audio"x, Video`, false, append(append([]model.Element{}, resource...), operandWant(model.ElementTypeResourcePath, "Video", "Video", 25)), nil},
		{`@resource-dir "C:\Audio\"`, true, append(append([]model.Element{}, resource...), operandWant(model.ElementTypeResourcePath, `"C:\Audio\"`, `C:\Audio\`, 15)), nil},
		{`@resource-dir "Audio, Video"`, true, append(append([]model.Element{}, resource...), operandWant(model.ElementTypeResourcePath, `"Audio, Video"`, "Audio, Video", 15)), nil},
		{"@resource-dir", true, resource, nil},
		{`@resource-dir ""`, true, append(append([]model.Element{}, resource...), operandWant(model.ElementTypeResourcePath, `""`, "", 15)), nil},
	} {
		t.Run(test.source, func(t *testing.T) {
			for _, orchestrated := range []bool{true, false} {
				context := newLineTestContext(t, test.source)
				before := context.Line()
				for _, detect := range registered.DetectionFuncs() {
					if !detect(context) {
						t.Fatal("Не завершено детектирование")
					}
				}
				var ok bool
				if orchestrated {
					ok = registered.OrchestrationFuncs()[0](context)
				} else {
					if !registered.LineTypeFuncs()[0](context) {
						t.Fatal("Не завершена классификация")
					}
					ok = registered.LineParserFuncs()[0](context)
				}
				if ok != test.ok || context.Line().LineType != model.LineTypeTag || len(context.Line().Elements) != len(test.elements) {
					t.Fatalf("Оркестрация=%t: ok=%t, требуется %t; строка %+v", orchestrated, ok, test.ok, context.Line())
				}
				for i, want := range test.elements {
					assertOperandElement(t, context.Line().Elements[i], want)
				}
				got := context.Diagnostics()
				if len(got) != len(test.diagnostics) {
					t.Fatalf("Диагностики %+v, требуются %+v", got, test.diagnostics)
				}
				for i, expected := range test.diagnostics {
					scope := diagnostics.ScopeLine
					if expected.code == diagnostics.P006 {
						scope = diagnostics.ScopeElement
					}
					location := model.Location{Start: model.Position{Line: 19, Column: expected.start}, End: model.Position{Line: 19, Column: expected.end}}
					if d := got[i]; d.DiagnosticCode != expected.code || d.DiagnosticScope != scope || d.Location == nil || *d.Location != location || d.Fatal || d.SeverityLevel != diagnostics.SeverityError {
						t.Fatalf("Неверная диагностика %+v, требуется %+v", d, expected)
					}
				}
				assertLineServiceFields(t, context, before)
			}
		})
	}
}

func TestRegisteredGrammarDiagnosticsThroughParser(t *testing.T) {
	// P015 добавляет парсер: проверяем её отсутствие у завершённого разбора
	// и сохранение у незакрытого SOURCE, в том числе после ошибки TYPE.
	for _, test := range []struct {
		source string
		kind   model.LineType
		codes  []diagnostics.Code
	}{
		{"#Title", model.LineTypeInvalid, []diagnostics.Code{diagnostics.P003}},
		{"  #### Ёж🌍  ", model.LineTypeInvalid, []diagnostics.Code{diagnostics.P003}},
		{"###\tTitle", model.LineTypeInvalid, []diagnostics.Code{diagnostics.P003}},
		{"###", model.LineTypeInvalid, []diagnostics.Code{diagnostics.P003}},
		{"@media", model.LineTypeTag, []diagnostics.Code{diagnostics.P006}},
		{"@media audio", model.LineTypeTag, []diagnostics.Code{diagnostics.P006}},
		{"@media vidio", model.LineTypeTag, []diagnostics.Code{diagnostics.P006, diagnostics.P006}},
		{`@media vidio "Unit 07"`, model.LineTypeTag, []diagnostics.Code{diagnostics.P006}},
		{`@media audio "Ёж🌍`, model.LineTypeTag, []diagnostics.Code{diagnostics.P015}},
		{`@media vidio "Ёж🌍`, model.LineTypeTag, []diagnostics.Code{diagnostics.P006, diagnostics.P015}},
	} {
		t.Run(test.source, func(t *testing.T) {
			registry := grammar.NewRegistry()
			checks := BuiltinGr12(registry)
			result := &model.Result{Format: model.FormatVersion1, Processing: []model.Processing{{ID: "current", Tool: "dsl-parser"}}}
			source := "@dsl-version 1.2\n" + test.source + "\n@document-id Next\nText"
			p := parser.New(strings.NewReader(source), result, registry, diagnostics.NewRegistry(), map[string]map[grammar.AssertionID]grammar.AssertionFunc{"1.2": checks})
			if err := p.Parse(); err != nil {
				t.Fatal(err)
			}
			if len(result.Lines) != 4 || len(result.Diagnostics) != len(test.codes) || !result.Document.HasErrors {
				t.Fatalf("Потеряны строки или неверен набор диагностик: %+v", result)
			}
			line := result.Lines[1]
			if line.LineType != test.kind || !line.HasErrors || line.Raw != test.source {
				t.Fatalf("Неверный результат ошибочной строки: %+v", line)
			}
			for i, code := range test.codes {
				d := result.Diagnostics[i]
				if d.DiagnosticCode != code || d.DiagnosticScope != diagnostics.ScopeElement || d.Location == nil || d.Location.Start.Line != 2 || d.Location.End.Line != 2 || d.Fatal || d.SeverityLevel != diagnostics.SeverityError {
					t.Fatalf("Требуется %s на второй строке: %+v", code, d)
				}
				matches := 0
				for _, element := range line.Elements {
					if element.ElementType == model.ElementTypeUnparsed && element.Start == d.Location.Start.Column && element.End == d.Location.End.Column && element.Value == nil && len(element.ErrorIDs) == 1 && element.ErrorIDs[0] == d.ID {
						matches++
					}
				}
				if matches != 1 {
					t.Fatalf("Нет однозначной связи %s с unparsed: %+v", code, line.Elements)
				}
			}
			if result.Document.Metadata.DocumentID == nil || *result.Document.Metadata.DocumentID != "Next" || result.Lines[0].HasErrors || result.Lines[2].HasErrors || result.Lines[3].HasErrors {
				t.Fatal("Ошибка помешала дальнейшему разбору или перенесена на соседние строки")
			}
			if result.Lines[2].LineType != model.LineTypeTag || len(result.Lines[2].Elements) != 2 || result.Lines[3].LineType != model.LineTypeContent || len(result.Lines[3].Elements) != 1 {
				t.Fatal("Изменён разбор последующих тега или содержимого")
			}
			assertOperandElement(t, result.Lines[2].Elements[1], operandWant(model.ElementTypeIdentifier, "Next", "Next", 14))
			assertOperandElement(t, result.Lines[3].Elements[0], operandWant(model.ElementTypeContent, "Text", "Text", 1))
		})
	}
}
