package gr12_test

import (
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/gr12"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestBuiltinGr12RegistersAndReplacesOnlyItsVersion(t *testing.T) {
	registry := grammar.NewRegistry()
	calls := 0
	action := func(grammar.GrammarContext) bool { calls++; return true }
	assertion := grammar.Assertion{ID: "other", Check: func(grammar.AssertionInput) bool { calls++; return true }}
	for _, version := range []string{"1.1", "1.2"} {
		registry.Register(version, []grammar.DetectionFunc{action}, []grammar.OrchestrationFunc{action},
			[]grammar.LineTypeFunc{action}, []grammar.LineParserFunc{action}, []grammar.Assertion{assertion})
	}
	for attempt := 0; attempt < 2; attempt++ {
		gr12.BuiltinGr12(registry)
		assertRegisteredGr12(t, registry)
		if calls != 0 || len(registry) != 2 {
			t.Fatal("Регистрация вызвала обработчики или изменила набор версий")
		}
		context := runRegisteredGr12(t, registry, "@dsl-version 1.2")
		if len(context.Line().Elements) != 2 || len(context.Diagnostics()) != 0 {
			t.Fatal("Повторная регистрация не заменила набор или добавила повторный разбор")
		}
		if calls != 0 {
			t.Fatal("Старый обработчик 1.2 остался в наборе")
		}
	}
	other, found := registry.Lookup("1.1")
	if !found || other.Version() != "1.1" || len(other.DetectionFuncs()) != 1 || len(other.OrchestrationFuncs()) != 1 || len(other.LineTypeFuncs()) != 1 || len(other.LineParserFuncs()) != 1 || len(other.Assertions()) != 1 || other.Assertions()[0].ID != "other" {
		t.Fatal("Изменён набор другой версии")
	}
	if !other.DetectionFuncs()[0](nil) || !other.OrchestrationFuncs()[0](nil) || !other.LineTypeFuncs()[0](nil) || !other.LineParserFuncs()[0](nil) || !other.Assertions()[0].Check(grammar.AssertionInput{}) || calls != 5 {
		t.Fatal("Не сохранены функции другой версии")
	}
	if _, found := grammar.NewRegistry().Lookup("1.2"); found {
		t.Fatal("Регистрация затронула независимый реестр")
	}
}

func TestBuiltinGr12RegisteredPipeline(t *testing.T) {
	registry := grammar.NewRegistry()
	gr12.BuiltinGr12(registry)
	assertRegisteredGr12(t, registry)
	for _, test := range []struct {
		source string
		kind   model.LineType
		types  []model.ElementType
		values []string
		code   diagnostics.Code
	}{
		// Примеры DSL 1.2, 1.21, 2.1 и синтетические варианты правил 1.18–1.19.
		{" \t", model.LineTypeBlank, nil, nil, ""},
		{"@dsl-version 1.2", model.LineTypeTag, []model.ElementType{model.ElementTypeTag, model.ElementTypeVersion}, []string{"dsl-version", "1.2"}, ""},
		{"# Unit 1", model.LineTypeHeading, []model.ElementType{model.ElementTypeHeadingLevel, model.ElementTypeTitle}, []string{"1", "Unit 1"}, ""},
		{" \t### Ёж🌍 \t", model.LineTypeHeading, []model.ElementType{model.ElementTypeHeadingLevel, model.ElementTypeTitle}, []string{"3", "Ёж🌍"}, ""},
		{`\@note This is ordinary text.`, model.LineTypeContent, []model.ElementType{model.ElementTypeContent}, []string{`\@note This is ordinary text.`}, ""},
		{"She _____{lives} in London.", model.LineTypeContent, []model.ElementType{model.ElementTypeContent}, []string{"She _____{lives} in London."}, ""},
		{"@NoTe Text @instr More", model.LineTypeTag, []model.ElementType{model.ElementTypeTag, model.ElementTypeContent}, []string{"note", "Text @instr More"}, ""},
		{"@note Tip {", model.LineTypeBlockStart, []model.ElementType{model.ElementTypeTag, model.ElementTypeTitle, model.ElementTypeBlockOpen}, []string{"note", "Tip", "{"}, ""},
		{"}", model.LineTypeBlockEnd, []model.ElementType{model.ElementTypeBlockClose}, []string{"}"}, ""},
		{"@noteText", model.LineTypeInvalid, []model.ElementType{model.ElementTypeUnparsed}, []string{""}, diagnostics.P003},
		{"{", model.LineTypeInvalid, []model.ElementType{model.ElementTypeUnparsed}, []string{""}, diagnostics.P008},
		{"@note Tip {x}", model.LineTypeBlockStart, []model.ElementType{model.ElementTypeTag, model.ElementTypeTitle, model.ElementTypeBlockOpen}, []string{"note", "Tip", "{"}, diagnostics.P008},
		{"} @note Text", model.LineTypeBlockEnd, []model.ElementType{model.ElementTypeBlockClose}, []string{"}"}, diagnostics.P010},
	} {
		t.Run(test.source, func(t *testing.T) {
			context := runRegisteredGr12(t, registry, test.source)
			line := context.Line()
			if line.LineType != test.kind || len(line.Elements) != len(test.types) {
				t.Fatalf("Неверный результат или повторный разбор: %+v", line)
			}
			for i, element := range line.Elements {
				if element.ElementType != test.types[i] || element.Start < 1 || element.End > utf8.RuneCountInString(test.source)+1 || element.End <= element.Start || element.Raw != string([]rune(test.source)[element.Start-1:element.End-1]) {
					t.Fatalf("Неверный элемент или исходный диапазон: %+v", element)
				}
				if test.types[i] == model.ElementTypeUnparsed {
					if element.Value != nil {
						t.Fatal("Выдумано значение unparsed")
					}
				} else if element.Value == nil || *element.Value != test.values[i] {
					t.Fatalf("Неверное значение элемента: %+v", element)
				}
			}
			got := context.Diagnostics()
			if test.code == "" {
				if len(got) != 0 {
					t.Fatalf("Лишние диагностики: %+v", got)
				}
			} else if len(got) != 1 || got[0].DiagnosticCode != test.code {
				t.Fatalf("Ожидалась одна диагностика %s: %+v", test.code, got)
			}
		})
	}
}

func TestBuiltinGr12ExposesSeparateClassificationAndParsing(t *testing.T) {
	registry := grammar.NewRegistry()
	gr12.BuiltinGr12(registry)
	assertRegisteredGr12(t, registry)
	registered, _ := registry.Lookup("1.2")
	for _, source := range []string{"@note Tip {", "@note Tip {x}"} {
		t.Run(source, func(t *testing.T) {
			context := grammar.NewContext(source)
			context.SetDiagnosticRegistry(diagnostics.NewRegistry())
			context.SetLine(model.Line{Number: 1, Raw: source})
			if !registered.LineTypeFuncs()[0](context) || context.Line().LineType != model.LineTypeTag || len(context.Line().Elements) != 0 || len(context.Diagnostics()) != 0 {
				t.Fatal("Классификация недоступна отдельно или запустила разбор")
			}
			if !registered.LineParserFuncs()[0](context) || context.Line().LineType != model.LineTypeBlockStart || len(context.Line().Elements) != 3 {
				t.Fatalf("Отдельный разбор не уточнил форму: %+v", context.Line())
			}
			want := runRegisteredGr12(t, registry, source)
			if !slices.EqualFunc(context.Line().Elements, want.Line().Elements, func(a, b model.Element) bool {
				return a.ElementType == b.ElementType && a.Raw == b.Raw && a.Start == b.Start && a.End == b.End && a.Value != nil && b.Value != nil && *a.Value == *b.Value
			}) || len(context.Diagnostics()) != len(want.Diagnostics()) {
				t.Fatal("Раздельные точки входа расходятся с оркестрацией нового контекста")
			}
		})
	}
}

func TestBuiltinGr12RegistersWorkingNamedAssertions(t *testing.T) {
	registry := grammar.NewRegistry()
	gr12.BuiltinGr12(registry)
	assertRegisteredGr12(t, registry)
	block := runRegisteredGr12(t, registry, "@note Tip {").Line()
	task := runRegisteredGr12(t, registry, "@task 1").Line()
	for _, test := range []struct {
		source   string
		id       grammar.AssertionID
		block    *model.Line
		parent   *model.Line
		expected bool
	}{
		{"@note Tip {", gr12.IncreasesNesting, nil, nil, true},
		{"@note Tip {x}", gr12.IncreasesNesting, nil, nil, true},
		{"@task 1", gr12.IncreasesNesting, nil, nil, true},
		{"@step Stage", gr12.IncreasesNesting, nil, nil, false},
		{"Text", gr12.IncreasesNesting, nil, nil, false},
		{"}", gr12.DecreasesNesting, &block, nil, true},
		{"} @note Text", gr12.DecreasesNesting, &block, &task, true},
		{"}", gr12.DecreasesNesting, nil, &task, false},
		{"@endtask", gr12.DecreasesNesting, nil, &task, true},
		{"@endtask", gr12.DecreasesNesting, nil, &block, false},
	} {
		t.Run(string(test.id)+"/"+test.source, func(t *testing.T) {
			context := runRegisteredGr12(t, registry, test.source)
			registered, _ := registry.Lookup("1.2")
			context.SetAssertions(registered.Assertions())
			input := grammar.AssertionInput{Line: context.Line(), OpenBlock: test.block, CandidateParent: test.parent}
			if got := context.Assert(test.id, input); got != test.expected {
				t.Fatalf("Assert(%s) = %t, требуется %t", test.id, got, test.expected)
			}
		})
	}
}

func assertRegisteredGr12(t *testing.T, registry grammar.Registry) {
	t.Helper()
	registered, found := registry.Lookup("1.2")
	if !found || registered.Version() != "1.2" {
		t.Fatal("Грамматика 1.2 не зарегистрирована")
	}
	if len(registered.DetectionFuncs()) != 1 || len(registered.OrchestrationFuncs()) != 1 || len(registered.LineTypeFuncs()) != 1 || len(registered.LineParserFuncs()) != 1 {
		t.Fatal("Неверный состав точек входа")
	}
	if registered.DetectionFuncs()[0] == nil || registered.OrchestrationFuncs()[0] == nil || registered.LineTypeFuncs()[0] == nil || registered.LineParserFuncs()[0] == nil {
		t.Fatal("Зарегистрирована отсутствующая функция")
	}
	assertions := registered.Assertions()
	if len(assertions) != 2 {
		t.Fatalf("Зарегистрировано %d утверждений вместо двух", len(assertions))
	}
	seen := make(map[grammar.AssertionID]bool)
	for _, assertion := range assertions {
		if assertion.Check == nil || seen[assertion.ID] || (assertion.ID != gr12.IncreasesNesting && assertion.ID != gr12.DecreasesNesting) {
			t.Fatalf("Неверное именованное утверждение: %+v", assertion)
		}
		seen[assertion.ID] = true
	}
}

func runRegisteredGr12(t *testing.T, registry grammar.Registry, source string) grammar.GrammarContext {
	t.Helper()
	registered, found := registry.Lookup("1.2")
	if !found {
		t.Fatal("Грамматика не зарегистрирована")
	}
	context := grammar.NewContext(source)
	context.SetDiagnosticRegistry(diagnostics.NewRegistry())
	context.SetLine(model.Line{Number: 1, Raw: source, LineEnding: model.LineEndingLF})
	for _, detect := range registered.DetectionFuncs() {
		if !detect(context) {
			t.Fatal("Зарегистрированное детектирование не завершено")
		}
	}
	if context.Line().LineType != "" || len(context.Line().Elements) != 0 || len(context.Diagnostics()) != 0 {
		t.Fatal("Детектирование преждевременно запустило разбор")
	}
	for _, orchestrate := range registered.OrchestrationFuncs() {
		if !orchestrate(context) {
			t.Fatal("Зарегистрированная оркестрация не завершена")
		}
	}
	if context.Line().Raw != source || context.String() != source || context.Line().Number != 1 || context.Line().LineEnding != model.LineEndingLF {
		t.Fatal("Утрачены исходник или служебные данные строки")
	}
	return context
}
