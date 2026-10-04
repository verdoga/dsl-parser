package gr12

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestReadTagNamePreservesCompleteNameAndUnicodeColumn(t *testing.T) {
	for _, test := range []struct {
		source, name string
		next         int
		ok           bool
	}{
		{"@dsl-version 1.2", "dsl-version", 13, true},
		{" \t@NoTe Text", "NoTe", 8, true},
		{"@noteText", "noteText", 10, true},
		{"@note{", "note", 6, true}, {"@note}", "note", 6, true},
		{"@note\tText", "note", 6, true},
		{`@resource-dir"Audio"`, "resource-dir", 14, true},
		{"@Неизвестный🌍 x", "Неизвестный🌍", 14, true},
		{"@note! x", "note!", 7, true}, {"@note@hint", "note@hint", 11, true},
		{`@note\{`, `note\{`, 8, true}, {`@note\\{`, `note\\`, 8, true},
		{"", "", 1, false}, {" \t", "", 3, false},
		{"@", "", 2, false}, {" \t@ Text", "", 4, false},
		{"@{", "", 2, false}, {`@"x"`, "", 2, false},
		{`\@note Text`, "", 1, false}, {` \\@note Text`, "", 2, false},
		{"Text @note", "", 1, false}, {"\u00a0@note", "", 1, false},
	} {
		t.Run(test.source, func(t *testing.T) {
			context := newOperandContext(t, test.source, "")
			before := syntaxLineSnapshot(t, context)
			name, next, ok := readTagName(context)
			if name != test.name || next != test.next || ok != test.ok {
				t.Errorf("readTagName() = %q, %d, %t; требуется %q, %d, %t", name, next, ok, test.name, test.next, test.ok)
			}
			if syntaxLineSnapshot(t, context) != before || context.String() != test.source || len(context.Diagnostics()) != 0 {
				t.Fatal("Чтение имени изменило контекст")
			}
		})
	}
}

func TestParseTagDispatchesAll34DSLNames(t *testing.T) {
	for _, test := range []struct {
		tag, suffix string
		block       bool
		parts       []tagPart
	}{
		// Примеры DSL 1.2, 1.32–1.35 и разделов 2–4.
		{"dsl-version", " 1.2", false, []tagPart{{model.ElementTypeVersion, "1.2", "1.2"}}},
		{"document-id", " scenario-7f3a91c2", false, []tagPart{{model.ElementTypeIdentifier, "scenario-7f3a91c2", "scenario-7f3a91c2"}}},
		{"section", " Studentbook", false, []tagPart{{model.ElementTypeName, "Studentbook", "Studentbook"}}},
		{"order", " 20", false, []tagPart{{model.ElementTypeNumber, "20", "20"}}},
		{"resource-dir", ` Audio, "../Shared resources", "../Video, additional"`, false, []tagPart{{model.ElementTypeResourcePath, "Audio", "Audio"}, {model.ElementTypeResourcePath, `"../Shared resources"`, "../Shared resources"}, {model.ElementTypeResourcePath, `"../Video, additional"`, "../Video, additional"}}},
		{"header", " Vocabulary practice", false, []tagPart{{model.ElementTypeTitle, "Vocabulary practice", "Vocabulary practice"}}},
		{"task", " task-a", false, []tagPart{{model.ElementTypeIdentifier, "task-a", "task-a"}}},
		{"endtask", "", false, nil},
		{"step", " Step 2. Read the text", false, []tagPart{{model.ElementTypeTitle, "Step 2. Read the text", "Step 2. Read the text"}}},
		{"speaking", "", false, nil}, {"newpage", "", false, nil},
		{"editor", " Check the wording before publication.", false, []tagPart{{model.ElementTypeContent, "Check the wording before publication.", "Check the wording before publication."}}},
		{"media", ` audio "dialogue tracks/track 01.mp3"`, false, []tagPart{{model.ElementTypeMediaType, "audio", "audio"}, {model.ElementTypeSource, `"dialogue tracks/track 01.mp3"`, "dialogue tracks/track 01.mp3"}}},
		{"example", " How to {", true, []tagPart{{model.ElementTypeTitle, "How to", "How to"}}},
		{"wordlist", " Italian; Spanish; British.", false, []tagPart{{model.ElementTypeContent, "Italian; Spanish; British.", "Italian; Spanish; British."}}},
		{"table", " Pronoun forms {", true, []tagPart{{model.ElementTypeTitle, "Pronoun forms", "Pronoun forms"}}},
		{"script", " Track 01 — Transcript {", true, []tagPart{{model.ElementTypeTitle, "Track 01 — Transcript", "Track 01 — Transcript"}}},
		{"text", " City Life {", true, []tagPart{{model.ElementTypeTitle, "City Life", "City Life"}}},
		{"key", " Answers {", true, []tagPart{{model.ElementTypeTitle, "Answers", "Answers"}}},
		{"instr", " {", true, nil},
		{"note", " Tip {", true, []tagPart{{model.ElementTypeTitle, "Tip", "Tip"}}},
		{"alt", " Улыбающаяся семья сидит на диване.", false, []tagPart{{model.ElementTypeContent, "Улыбающаяся семья сидит на диване.", "Улыбающаяся семья сидит на диване."}}},
		{"hint", " Remember to use the third-person singular form.", false, []tagPart{{model.ElementTypeContent, "Remember to use the third-person singular form.", "Remember to use the third-person singular form."}}},
		{"fragment", " grammar-note {", true, []tagPart{{model.ElementTypeIdentifier, "grammar-note", "grammar-note"}}},
		{"include", " grammar-note", false, []tagPart{{model.ElementTypeIdentifier, "grammar-note", "grammar-note"}}},
		{"answer", " New York", false, []tagPart{{model.ElementTypeContent, "New York", "New York"}}},
		{"question", " What is your name?", false, []tagPart{{model.ElementTypeContent, "What is your name?", "What is your name?"}}},
		{"multifill", " Complete the text. {", true, []tagPart{{model.ElementTypeContent, "Complete the text.", "Complete the text."}}},
		{"choice", " Choose a country. {", true, []tagPart{{model.ElementTypeContent, "Choose a country.", "Choose a country."}}},
		{"multichoice", " Select your favourite activities. {", true, []tagPart{{model.ElementTypeContent, "Select your favourite activities.", "Select your favourite activities."}}},
		{"matching", " Answer each question. {", true, []tagPart{{model.ElementTypeContent, "Answer each question.", "Answer each question."}}},
		{"ordering", " Put the stages in order. {", true, []tagPart{{model.ElementTypeContent, "Put the stages in order.", "Put the stages in order."}}},
		{"variants", " {", true, nil},
		{"variant", " Student A", false, []tagPart{{model.ElementTypeName, "Student A", "Student A"}}},
	} {
		mixed := []rune(test.tag)
		for i, symbol := range mixed {
			if i%2 == 0 && symbol >= 'a' && symbol <= 'z' {
				mixed[i] -= 'a' - 'A'
			}
		}
		for _, spelling := range []string{test.tag, strings.ToUpper(test.tag), string(mixed)} {
			t.Run(spelling, func(t *testing.T) {
				source := " \t@" + spelling + test.suffix + " \t"
				context := newOperandContext(t, source, "")
				line := context.Line()
				parent := 4
				line.LineType, line.ParentLine, line.NestingLevel = model.LineTypeInvalid, &parent, 2
				context.SetLine(line)
				parseTag(context)
				parts := []tagPart{{model.ElementTypeTag, "@" + spelling, test.tag}}
				parts = append(parts, test.parts...)
				wantType := model.LineTypeTag
				if test.block {
					wantType = model.LineTypeBlockStart
					parts = append(parts, tagPart{model.ElementTypeBlockOpen, "{", "{"})
				}
				got := context.Line()
				if got.LineType != wantType || got.Raw != source || context.String() != source || got.Number != 19 || got.ParentLine != &parent || parent != 4 || got.NestingLevel != 2 || got.HasErrors {
					t.Fatalf("Неверные свойства строки: %+v", got)
				}
				if len(got.Elements) != len(parts) || len(context.Diagnostics()) != 0 {
					t.Fatalf("Неверный результат диспетчеризации: %+v, диагностики %+v", got.Elements, context.Diagnostics())
				}
				cursor := 0
				for i, part := range parts {
					index := strings.Index(source[cursor:], part.raw)
					if index < 0 {
						t.Fatalf("Ожидаемый фрагмент %q отсутствует в примере", part.raw)
					}
					start := utf8.RuneCountInString(source[:cursor+index]) + 1
					assertOperandElement(t, got.Elements[i], operandWant(part.kind, part.raw, part.value, start))
					cursor += index + len(part.raw)
				}
			})
		}
	}
}

func TestParseTagRejectsUnknownCompleteNames(t *testing.T) {
	for _, test := range []struct {
		source, raw string
		start, end  int
	}{
		{"@noteText", "@noteText", 1, 10}, {"@unknown value {", "@unknown", 1, 9},
		{" \t@UnKnOwN @note Text", "@UnKnOwN", 3, 11},
		{"@Неизвестный🌍 x", "@Неизвестный🌍", 1, 14},
		{"@note! x", "@note!", 1, 7}, {"@note@hint", "@note@hint", 1, 11},
		{"@", "@", 1, 2}, {" \t@ {", "@", 3, 4},
	} {
		t.Run(test.source, func(t *testing.T) {
			context := newOperandContext(t, test.source, "")
			parseTag(context)
			line := context.Line()
			if line.LineType != model.LineTypeInvalid || line.Raw != test.source || context.String() != test.source || len(line.Elements) != 1 {
				t.Fatalf("Неизвестный тег разобран как известная конструкция: %+v", line)
			}
			assertOperandElement(t, line.Elements[0], model.Element{ElementType: model.ElementTypeUnparsed, Raw: test.raw, Start: test.start, End: test.end})
			got := context.Diagnostics()
			if len(got) != 1 {
				t.Fatalf("Диагностики %+v, требуется одна P003", got)
			}
			d := got[0]
			want := model.Location{Start: model.Position{Line: 19, Column: test.start}, End: model.Position{Line: 19, Column: test.end}}
			if d.DiagnosticCode != diagnostics.P003 || d.DiagnosticScope != diagnostics.ScopeElement || d.Location == nil || *d.Location != want || d.ID != "" || d.Source != "" {
				t.Errorf("P003 не связана с диапазоном неизвестного имени: %+v", d)
			}
		})
	}
}

func TestParseTagSeparatesAttachedBraceAndKeepsInternalAtSign(t *testing.T) {
	context := newOperandContext(t, "@note{", "")
	parseTag(context)
	line := context.Line()
	if line.LineType != model.LineTypeBlockStart || len(line.Elements) != 2 {
		t.Fatalf("Прилепленная скобка не отделена от известного тега: %+v", line)
	}
	assertOperandElement(t, line.Elements[0], operandWant(model.ElementTypeTag, "@note", "note", 1))
	assertOperandElement(t, line.Elements[1], operandWant(model.ElementTypeBlockOpen, "{", "{", 6))
	got := context.Diagnostics()
	want := model.Location{Start: model.Position{Line: 19, Column: 6}, End: model.Position{Line: 19, Column: 7}}
	if len(got) != 1 || got[0].DiagnosticCode != diagnostics.P004 || got[0].DiagnosticScope != diagnostics.ScopeLine || got[0].Location == nil || *got[0].Location != want {
		t.Fatalf("Требуется только P004 у разделителя: %+v", got)
	}
	for _, tag := range []string{"section", "header", "step", "question", "variant", "editor", "instr", "hint", "answer", "example", "wordlist", "note", "alt", "multifill"} {
		t.Run(tag, func(t *testing.T) {
			text := "Ёж🌍 @note Text"
			context := newOperandContext(t, "@"+tag+" "+text, "")
			parseTag(context)
			line := context.Line()
			if len(line.Elements) != 2 || line.LineType != model.LineTypeTag || len(context.Diagnostics()) != 0 {
				t.Fatalf("@ внутри значения начал новую конструкцию: %+v", line)
			}
			kind := model.ElementTypeContent
			if tag == "section" || tag == "variant" {
				kind = model.ElementTypeName
			} else if tag == "header" || tag == "step" {
				kind = model.ElementTypeTitle
			}
			assertOperandElement(t, line.Elements[1], operandWant(kind, text, text, len(tag)+3))
		})
	}
}

func TestParseTagDispatchesAlternativeForms(t *testing.T) {
	for _, tag := range []string{"editor", "instr", "hint", "answer", "example", "wordlist", "note", "alt", "multifill"} {
		for _, suffix := range []string{"", " {", " Title {"} {
			t.Run(tag+suffix, func(t *testing.T) {
				context := newOperandContext(t, "@"+tag+suffix, "")
				parseTag(context)
				parts := []model.Element{operandWant(model.ElementTypeTag, "@"+tag, tag, 1)}
				lineType := model.LineTypeTag
				forbiddenTitle := false
				if suffix == " Title {" {
					forbiddenTitle = tag == "editor" || tag == "instr" || tag == "hint" || tag == "answer"
					if !forbiddenTitle {
						kind := model.ElementTypeTitle
						if tag == "multifill" {
							kind = model.ElementTypeContent
						}
						parts = append(parts, operandWant(kind, "Title", "Title", len(tag)+3))
					}
				}
				if suffix != "" {
					lineType = model.LineTypeBlockStart
					parts = append(parts, operandWant(model.ElementTypeBlockOpen, "{", "{", 1+len(tag)+len(suffix)))
				}
				line, errors := context.Line(), context.Diagnostics()
				if line.LineType != lineType || len(line.Elements) != len(parts) {
					t.Fatalf("Неверная альтернативная форма: %+v", line)
				}
				for i, part := range parts {
					assertOperandElement(t, line.Elements[i], part)
				}
				if (!forbiddenTitle && len(errors) != 0) || (forbiddenTitle && (len(errors) != 1 || errors[0].DiagnosticCode != diagnostics.P007)) {
					t.Fatalf("Диагностики альтернативной формы: %+v", errors)
				}
			})
		}
	}
}

func TestTagParsingIgnoresNonTagInput(t *testing.T) {
	parseTag(nil)
	if name, next, ok := readTagName(nil); name != "" || next != 1 || ok {
		t.Fatalf("readTagName(nil) = %q, %d, %t", name, next, ok)
	}
	for _, source := range []string{"", " \t", `\@note Text`, ` \\@note Text`, "Text @note", "# Title", "\u00a0@note"} {
		t.Run(source, func(t *testing.T) {
			context := grammar.NewContext(source)
			context.SetLine(model.Line{Number: 19, Raw: source, LineType: model.LineTypeContent})
			before := syntaxLineSnapshot(t, context)
			parseTag(context)
			if syntaxLineSnapshot(t, context) != before || len(context.Diagnostics()) != 0 || context.String() != source {
				t.Fatal("Строка без начального тега была изменена")
			}
		})
	}
}

// tagPart задаёт наблюдаемый результат диспетчеризации, независимо от обработчика.
type tagPart struct {
	kind       model.ElementType
	raw, value string
}
