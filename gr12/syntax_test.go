package gr12

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestCheckSeparatorAtDeclarationBoundary(t *testing.T) {
	for _, test := range []struct {
		source      string
		column, end int
		want        bool
	}{
		// DSL 1.2, 3.1.2, 3.12.3: исходные примеры и замена одного разделителя.
		{"@dsl-version 1.2", 13, 0, true}, {"@dsl-version\t1.2", 13, 14, false},
		{`@media audio "dialogue tracks/track 01.mp3"`, 7, 0, true},
		{`@media audio "dialogue tracks/track 01.mp3"`, 13, 0, true},
		{"@media audio\t12", 13, 14, false}, {"@fragment grammar-note {", 23, 0, true},
		{"@note Text", 6, 0, true}, {"@note   Text", 6, 0, true},
		{"@note Привет\tмир", 6, 0, true}, {"@note{", 6, 7, false},
		{"@note\tText", 6, 7, false}, {"@note\u00a0Text", 6, 7, false},
		{"@note", 6, 6, false}, {"Ж🌍\t{", 3, 4, false},
	} {
		t.Run(test.source, func(t *testing.T) {
			context := newSyntaxContext(t, test.source)
			if got := checkSeparator(context, test.column); got != test.want {
				t.Errorf("checkSeparator() = %t, требуется %t", got, test.want)
			}
			code := diagnostics.P004
			if test.want {
				code = ""
			}
			assertSyntaxDiagnostic(t, context, code, diagnostics.ScopeLine, test.column, test.end)
		})
	}
}

func TestCheckRequiredOperandUsesInsertionPosition(t *testing.T) {
	for _, test := range []struct {
		source  string
		present bool
		column  int
	}{
		{"@task ID", true, 7}, {"@task", false, 6}, {"@fragment {", false, 11}, {"Ж🌍", false, 3},
	} {
		t.Run(test.source, func(t *testing.T) {
			context := newSyntaxContext(t, test.source)
			if got := checkRequiredOperand(context, test.present, test.column); got != test.present {
				t.Errorf("checkRequiredOperand() = %t, требуется %t", got, test.present)
			}
			code := diagnostics.P006
			if test.present {
				code = ""
			}
			assertSyntaxDiagnostic(t, context, code, diagnostics.ScopeLine, test.column, test.column)
		})
	}
}

func TestCheckAllowedFormForEveryTag(t *testing.T) {
	// Литералы взяты из примеров указанных разделов DSL v1.2.md.
	// Недостающая форма синтезируется добавлением или удалением конечной " {".
	// Здесь проверяется разрешение формы по имени; её операнды проверяют другие функции.
	for _, test := range []struct {
		section, inline, block  string
		allowInline, allowBlock bool
	}{
		{"1.2", "@dsl-version 1.2", "", true, false},
		{"1.32", "@document-id scenario-7f3a91c2", "", true, false},
		{"1.33", "@section Studentbook", "", true, false},
		{"1.34", "@order 20", "", true, false},
		{"1.35", `@resource-dir Audio, "../Shared resources", "../Video, additional"`, "", true, false},
		{"2.2", "@header Vocabulary practice", "", true, false},
		{"2.3", "@task task-a", "", true, false},
		{"2.4", "@endtask", "", true, false},
		{"2.5", "@step Step 2. Read the text", "", true, false},
		{"2.6", "@speaking", "", true, false},
		{"2.7", "@newpage", "", true, false},
		{"2.8", "@editor Check the wording before publication.", "@editor {", true, true},
		{"3.1", `@media audio "dialogue tracks/track 01.mp3"`, "", true, false},
		{"3.2", "@example He live in London - He lives in London.", "@example How to {", true, true},
		{"3.3", "@wordlist Italian; Spanish; British.", "@wordlist Nationalities {", true, true},
		{"3.4", "", "@table Pronoun forms {", false, true},
		{"3.5", "", "@script Track 01 — Transcript {", false, true},
		{"3.6", "", "@text City Life {", false, true},
		{"3.7", "", "@key Answers {", false, true},
		{"3.8", "@instr Read the text and answer the questions.", "@instr {", true, true},
		{"3.9", "@note Remember to use capital letters.", "@note Tip {", true, true},
		{"3.10", "@alt Улыбающаяся семья сидит на диване.", "@alt Изображение 4A {", true, true},
		{"3.11", "@hint Remember to use the third-person singular form.", "@hint {", true, true},
		{"3.12", "", "@fragment grammar-note {", false, true},
		{"3.13", "@include grammar-note", "", true, false},
		{"4.1", "@answer New York", "@answer {", true, true},
		{"4.2", "@question What is your name?", "", true, false},
		{"4.4/no instruction", "@multifill", "@multifill {", true, true},
		{"4.4/instruction", "@multifill Write your answer.", "@multifill Complete the text. {", true, true},
		{"4.5", "", "@choice Choose a country. {", false, true},
		{"4.6", "", "@multichoice Select your favourite activities. {", false, true},
		{"4.7", "", "@matching Answer each question. {", false, true},
		{"4.8", "", "@ordering Put the stages in order. {", false, true},
		{"4.9/variants", "", "@variants {", false, true},
		{"4.9/variant", "@variant Student A", "", true, false},
		{"1.16/unknown", "@unknown", "", false, false},
		{"1.29/unknown prefix", "@noteText", "", false, false},
	} {
		for _, hasOpen := range []bool{false, true} {
			t.Run(fmt.Sprintf("DSL %s/block=%t", test.section, hasOpen), func(t *testing.T) {
				inline, block := test.inline, test.block
				if inline == "" {
					inline = strings.TrimSuffix(block, " {")
				}
				if block == "" {
					block = inline + " {"
				}
				source, want := inline, test.allowInline
				if hasOpen {
					source, want = block, test.allowBlock
				}
				tag := strings.TrimPrefix(strings.Fields(source)[0], "@")
				context := newSyntaxContext(t, " \t"+source+" \t")
				if got := checkAllowedForm(context, strings.ToUpper(tag), hasOpen); got != want {
					t.Errorf("checkAllowedForm() = %t, требуется %t", got, want)
				}
				code := diagnostics.P005
				if want || (!test.allowInline && !test.allowBlock) {
					code = ""
				}
				assertSyntaxDiagnostic(t, context, code, diagnostics.ScopeLine, 3, utf8.RuneCountInString(source)+3)
			})
		}
	}
}

func TestSyntaxTailChecks(t *testing.T) {
	for _, test := range []struct {
		name, source string
		check        func(grammar.GrammarContext, int)
		column       int
		code         diagnostics.Code
		scope        diagnostics.Scope
		start, end   int
	}{
		{"DSL 3.13 valid include", "@include grammar-note", checkTagTail, 22, "", "", 0, 0},
		{"DSL 3.13 extra operand", "@include grammar-note extra", checkTagTail, 22, diagnostics.P007, diagnostics.ScopeLine, 23, 28},
		{"DSL 3.6 valid opening", "@text City Life {", checkOpeningTail, 17, "", "", 0, 0},
		{"DSL 3.6 opening with content", "@text City Life { Text", checkOpeningTail, 17, diagnostics.P008, diagnostics.ScopeLine, 17, 23},
		{"tag empty", "@endtask", checkTagTail, 9, "", "", 0, 0},
		{"tag whitespace", "@include часть \t", checkTagTail, 15, "", "", 0, 0},
		{"tag extra tokens", "@include часть extra \t", checkTagTail, 15, diagnostics.P007, diagnostics.ScopeLine, 16, 21},
		{"tag Unicode tail", "@include x \tёж🌍 \t", checkTagTail, 11, diagnostics.P007, diagnostics.ScopeLine, 13, 16},
		{"opening empty tail", "@text {", checkOpeningTail, 7, "", "", 0, 0},
		{"opening whitespace tail", "@text { \t", checkOpeningTail, 7, "", "", 0, 0},
		{"standalone opening", "{", checkOpeningTail, 1, diagnostics.P008, diagnostics.ScopeElement, 1, 2},
		{"standalone opening with edges", " \t{ \t", checkOpeningTail, 3, diagnostics.P008, diagnostics.ScopeElement, 3, 4},
		{"opening with content", "@text { 😀 \t", checkOpeningTail, 7, diagnostics.P008, diagnostics.ScopeLine, 7, 10},
		{"opening with closing", "@text { }", checkOpeningTail, 7, diagnostics.P008, diagnostics.ScopeLine, 7, 10},
		{"standalone opening with content", "{ хвост", checkOpeningTail, 1, diagnostics.P008, diagnostics.ScopeLine, 1, 8},
		{"closing only", "}", checkClosingTail, 1, "", "", 0, 0},
		{"closing with edges", "\t } \t", checkClosingTail, 3, "", "", 0, 0},
		{"closing with tag", "} @note X", checkClosingTail, 1, diagnostics.P010, diagnostics.ScopeLine, 1, 10},
		{"two closings", "} }", checkClosingTail, 1, diagnostics.P010, diagnostics.ScopeLine, 1, 4},
		{"content before closing", "ёж }  ", checkClosingTail, 4, diagnostics.P010, diagnostics.ScopeLine, 1, 5},
		{"nonbreaking space is content", "}\u00a0", checkClosingTail, 1, diagnostics.P010, diagnostics.ScopeLine, 1, 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			context := newSyntaxContext(t, test.source)
			before := syntaxLineSnapshot(t, context)
			test.check(context, test.column)
			assertSyntaxDiagnostic(t, context, test.code, test.scope, test.start, test.end)
			if context.String() != test.source || syntaxLineSnapshot(t, context) != before {
				t.Fatal("Проверка хвоста изменила исходник или результат разбора строки")
			}
		})
	}
}

func TestCheckStrayBraceRespectsRangeAndEscaping(t *testing.T) {
	for _, test := range []struct {
		source     string
		start, end int
		wantError  bool
	}{
		// DSL 1.21: исходный пример и удаление одного экранирующего символа.
		{`Use \{ and \} as text.`, 1, 23, false}, {`Use { and \} as text.`, 1, 22, true},
		{`@note "{x}"`, 7, 12, true}, {`{}`, 1, 3, true}, {`\{x\}`, 1, 6, false},
		{`\{`, 2, 3, false}, {`\\{`, 3, 4, true}, {`\\\{`, 4, 5, false},
		{`\\}`, 3, 4, true}, {`\Ж{`, 3, 4, true}, {`{} текст {`, 4, 9, false},
		{"🌍{", 2, 3, true}, {"{", 1, 1, false}, {"", 1, 1, false},
	} {
		t.Run(test.source, func(t *testing.T) {
			context := newSyntaxContext(t, test.source)
			value := string([]rune(test.source)[test.start-1 : test.end-1])
			addElement(context, model.ElementTypeContent, test.start, test.end, &value)
			before := syntaxLineSnapshot(t, context)
			checkStrayBrace(context, test.start, test.end)
			code := diagnostics.P012
			if !test.wantError {
				code = ""
			}
			assertSyntaxDiagnostic(t, context, code, diagnostics.ScopeElement, test.start, test.end)
			if syntaxLineSnapshot(t, context) != before {
				t.Fatal("Проверка скобок изменила элементы строки")
			}
		})
	}
}

func TestAddElementPreservesUnicodeFragmentAndSuppliedValue(t *testing.T) {
	for _, test := range []struct {
		source     string
		start, end int
		raw        string
	}{
		{" \tА🌍е\u0301\\{ \t", 3, 9, "А🌍е\u0301\\{"},
		{" \tА🌍е\u0301\\{ \t", 4, 5, "🌍"},
		{" \tА🌍е\u0301\\{ \t", 6, 7, "\u0301"},
		{" \tА🌍е\u0301\\{ \t", 11, 11, ""},
		// DSL 1.21: текст и экранирование сохраняются буквально.
		{`Use \{ and \} as text.`, 1, 23, `Use \{ and \} as text.`},
		// DSL 3.1.2: запись уже выделенного SOURCE не снимает кавычки.
		{`@media audio "dialogue tracks/track 01.mp3"`, 14, 44, `"dialogue tracks/track 01.mp3"`},
	} {
		for _, hasValue := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d:%d/value=%t", test.start, test.end, hasValue), func(t *testing.T) {
				context := newSyntaxContext(t, test.source)
				addElement(context, model.ElementTypeUnparsed, 1, 2, nil)
				text := `Сохранить \{ и @TAG`
				var value *string
				if hasValue {
					value = &text
				}
				addElement(context, model.ElementTypeContent, test.start, test.end, value)
				line := context.Line()
				if len(line.Elements) != 2 || line.Elements[0].Raw != string([]rune(test.source)[:1]) || line.Number != 7 || line.Raw != context.String() {
					t.Fatal("Потеряны данные строки или ранее добавленный элемент")
				}
				element := line.Elements[1]
				if element.ElementType != model.ElementTypeContent || element.Raw != test.raw || element.Start != test.start || element.End != test.end || element.Value != value || len(element.ErrorIDs) != 0 {
					t.Errorf("Неверный элемент: %+v", element)
				}
				if text != `Сохранить \{ и @TAG` || len(context.Diagnostics()) != 0 {
					t.Fatal("Значение интерпретировано повторно или добавлена диагностика")
				}
			})
		}
	}
}

func TestAddSyntaxDiagnosticCopiesRegistryDescriptionAndAccumulates(t *testing.T) {
	context := newSyntaxContext(t, "@text { x")
	before := syntaxLineSnapshot(t, context)
	codes := []diagnostics.Code{diagnostics.P001, diagnostics.P003, diagnostics.P007, diagnostics.P008, diagnostics.P012}
	for _, code := range codes {
		location := model.Location{Start: model.Position{Line: 7, Column: 7}, End: model.Position{Line: 7, Column: 10}}
		addSyntaxDiagnostic(context, code, location)
		location.Start.Column = 99
	}
	got := context.Diagnostics()
	if len(got) != len(codes) {
		t.Fatalf("Диагностик %d, требуется %d", len(got), len(codes))
	}
	for i, code := range codes {
		description, _ := context.Lookup(code)
		d := got[i]
		if d.DiagnosticCode != code || d.Message != description.Message() || d.SeverityLevel != description.Severity() || d.DiagnosticScope != description.Scope() || d.Fatal != description.IsFatal() || d.ID != "" || d.Source != "" || len(d.RelatedLocations) != 0 {
			t.Errorf("Неверные свойства диагностики: %+v", d)
		}
		if d.Location == nil || *d.Location != (model.Location{Start: model.Position{Line: 7, Column: 7}, End: model.Position{Line: 7, Column: 10}}) {
			t.Errorf("Неверный диапазон диагностики: %+v", d.Location)
		}
	}
	if syntaxLineSnapshot(t, context) != before {
		t.Fatal("Запись диагностики изменила строку")
	}
}

func TestSyntaxChecksIgnoreUnavailableInput(t *testing.T) {
	for _, source := range []string{"", " \t", "🌍"} {
		t.Run(source, func(t *testing.T) {
			context := newSyntaxContext(t, source)
			before := syntaxLineSnapshot(t, context)
			for _, input := range []grammar.GrammarContext{nil, context} {
				for _, column := range []int{-1, 0, utf8.RuneCountInString(source) + 2} {
					if checkSeparator(input, column) || checkRequiredOperand(input, false, column) {
						t.Fatal("Невозможная позиция подтверждена как разделитель или операнд")
					}
					checkTagTail(input, column)
					checkOpeningTail(input, column)
					checkClosingTail(input, column)
				}
				for _, bounds := range [][2]int{{0, 1}, {2, 1}, {1, utf8.RuneCountInString(source) + 2}} {
					checkStrayBrace(input, bounds[0], bounds[1])
					addElement(input, model.ElementTypeContent, bounds[0], bounds[1], nil)
				}
				checkOpeningTail(input, 1)
				checkClosingTail(input, 1)
				addSyntaxDiagnostic(input, diagnostics.Code("unregistered"), model.Location{})
			}
			if syntaxLineSnapshot(t, context) != before || len(context.Diagnostics()) != 0 {
				t.Fatal("Недоступный вход изменил результат разбора")
			}
		})
	}
}

func newSyntaxContext(t *testing.T, source string) grammar.GrammarContext {
	t.Helper()
	context := grammar.NewContext(source)
	context.SetLine(model.Line{Number: 7, Raw: source, LineType: model.LineTypeTag})
	context.SetDiagnosticRegistry(diagnostics.NewRegistry())
	return context
}

func assertSyntaxDiagnostic(t *testing.T, context grammar.GrammarContext, code diagnostics.Code, scope diagnostics.Scope, start, end int) {
	t.Helper()
	got := context.Diagnostics()
	if code == "" {
		if len(got) != 0 {
			t.Fatalf("Неожиданные диагностики: %+v", got)
		}
		return
	}
	if len(got) != 1 {
		t.Fatalf("Диагностик %d, требуется одна %s", len(got), code)
	}
	d := got[0]
	want := model.Location{Start: model.Position{Line: 7, Column: start}, End: model.Position{Line: 7, Column: end}}
	if d.DiagnosticCode != code || d.DiagnosticScope != scope || d.Location == nil || *d.Location != want || d.ID != "" || d.Source != "" {
		t.Errorf("Диагностика %+v, диапазон %+v; требуется %s/%s, %+v", d, d.Location, code, scope, want)
	}
}

func syntaxLineSnapshot(t *testing.T, context grammar.GrammarContext) string {
	t.Helper()
	encoded, err := json.Marshal(context.Line())
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
