package gr12

import (
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
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

func TestOrchestrateLineReportsIncompleteProcessing(t *testing.T) {
	if orchestrateLine(nil) {
		t.Fatal("Отсутствующий контекст принят")
	}
	for _, source := range []string{"#Title", "#### Title", "###\tTitle"} {
		t.Run(source, func(t *testing.T) {
			context := newLineTestContext(t, source)
			detectLine(context)
			before := context.Line()
			if orchestrateLine(context) || context.Line().LineType != model.LineTypeInvalid {
				t.Fatal("Незавершённый разбор не отражён в результате")
			}
			if len(context.Line().Elements) != 0 || len(context.Diagnostics()) != 0 {
				t.Fatal("Созданы неподтверждённые элементы или диагностики")
			}
			assertLineServiceFields(t, context, before)
		})
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
	resource := []model.Element{operandWant(model.ElementTypeTag, "@resource-dir", "resource-dir", 1)}
	for _, test := range []struct {
		source   string
		ok       bool
		elements []model.Element
		code     diagnostics.Code
	}{
		{`@media audio "unclosed`, false, media, ""},
		{`@media audio "a\"`, false, media, ""},
		{`@media audio "Ёж🌍`, false, media, ""},
		{"@media\taudio \"unclosed", false, media, diagnostics.P004},
		{`@media audio "a\"b"`, true, append(append([]model.Element{}, media...), operandWant(model.ElementTypeSource, `"a\"b"`, `a\"b`, 14)), ""},
		{"@media audio", true, media, diagnostics.P006},
		{`@resource-dir "unclosed`, false, resource, ""},
		{`@resource-dir Audio, "unclosed`, false, append(append([]model.Element{}, resource...), operandWant(model.ElementTypeResourcePath, "Audio", "Audio", 15)), ""},
		{`@resource-dir Ёж🌍, "unclosed`, false, append(append([]model.Element{}, resource...), operandWant(model.ElementTypeResourcePath, "Ёж🌍", "Ёж🌍", 15)), ""},
		{`@resource-dir "Audio"x, Video`, false, append(append([]model.Element{}, resource...), operandWant(model.ElementTypeResourcePath, "Video", "Video", 25)), ""},
		{`@resource-dir "C:\Audio\"`, true, append(append([]model.Element{}, resource...), operandWant(model.ElementTypeResourcePath, `"C:\Audio\"`, `C:\Audio\`, 15)), ""},
		{`@resource-dir "Audio, Video"`, true, append(append([]model.Element{}, resource...), operandWant(model.ElementTypeResourcePath, `"Audio, Video"`, "Audio, Video", 15)), ""},
		{"@resource-dir", true, resource, ""},
		{`@resource-dir ""`, true, append(append([]model.Element{}, resource...), operandWant(model.ElementTypeResourcePath, `""`, "", 15)), ""},
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
				if test.code == "" {
					if len(got) != 0 {
						t.Fatalf("Добавлены несогласованные диагностики: %+v", got)
					}
				} else if len(got) != 1 || got[0].DiagnosticCode != test.code {
					t.Fatalf("Утрачена или продублирована диагностика %s: %+v", test.code, got)
				}
				assertLineServiceFields(t, context, before)
			}
		})
	}
}
