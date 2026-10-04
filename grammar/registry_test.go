package grammar_test

import (
	"slices"
	"testing"

	"github.com/verdoga/dsl-parser/grammar"
)

func TestRegistryFindsOnlyExactRegisteredVersions(t *testing.T) {
	r := grammar.NewRegistry()
	for _, version := range []string{"", "1.2", "unknown"} {
		if _, found := r.Lookup(version); found {
			t.Fatalf("Новый реестр содержит версию %q", version)
		}
	}
	versions := []string{"", "1.2", " 1.2 ", "DSL-next", "dsl-next"}
	for _, version := range versions {
		r.Register(version, nil, nil, nil, nil, []grammar.Assertion{{ID: grammar.AssertionID(version)}})
	}
	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			g, found := r.Lookup(version)
			if !found || g.Version() != version {
				t.Fatalf("Lookup(%q): версия = %q, found = %t", version, g.Version(), found)
			}
			assertions := g.Assertions()
			if len(assertions) != 1 || string(assertions[0].ID) != version {
				t.Fatalf("Для версии %q получен чужой набор: %+v", version, assertions)
			}
			if len(g.DetectionFuncs()) != 0 || len(g.OrchestrationFuncs()) != 0 ||
				len(g.LineTypeFuncs()) != 0 || len(g.LineParserFuncs()) != 0 {
				t.Fatal("Пустые наборы функций не сохранены")
			}
		})
	}
	for _, version := range []string{"unknown", "1.2 ", "1.2\n", "Dsl-next"} {
		if _, found := r.Lookup(version); found {
			t.Fatalf("Незарегистрированный ключ %q найден", version)
		}
	}
	if _, found := grammar.NewRegistry().Lookup("1.2"); found {
		t.Fatal("Независимые реестры разделяют регистрации")
	}
}

func TestRegistryCopiesAllInputSlicesWithoutCallingFunctions(t *testing.T) {
	var calls []string
	detection := registryTestActions[grammar.DetectionFunc]("detection", &calls)
	orchestration := registryTestActions[grammar.OrchestrationFunc]("orchestration", &calls)
	lineType := registryTestActions[grammar.LineTypeFunc]("line type", &calls)
	lineParser := registryTestActions[grammar.LineParserFunc]("line parser", &calls)
	assertions := []grammar.Assertion{
		{ID: "duplicate", Check: func(grammar.AssertionInput) bool {
			calls = append(calls, "assertion first")
			return true
		}},
		{ID: "missing"},
		{ID: "duplicate", Check: func(grammar.AssertionInput) bool {
			calls = append(calls, "assertion second")
			return false
		}},
	}
	r := grammar.NewRegistry()
	r.Register("1.2", detection, orchestration, lineType, lineParser, assertions)
	if len(calls) != 0 {
		t.Fatal("Register вызвал функции")
	}
	clear(detection)
	clear(orchestration)
	clear(lineType)
	clear(lineParser)
	clear(assertions)
	for attempt := 0; attempt < 2; attempt++ {
		calls = nil
		g, found := r.Lookup("1.2")
		if !found || len(calls) != 0 {
			t.Fatal("Lookup не нашёл версию или вызвал функции")
		}
		d, o, lt, lp, a := g.DetectionFuncs(), g.OrchestrationFuncs(), g.LineTypeFuncs(), g.LineParserFuncs(), g.Assertions()
		if len(calls) != 0 {
			t.Fatal("Получение наборов вызвало функции")
		}
		assertRegistryActions(t, d)
		assertRegistryActions(t, o)
		assertRegistryActions(t, lt)
		assertRegistryActions(t, lp)
		if len(a) != 3 || a[0].ID != "duplicate" || a[1].ID != "missing" || a[2].ID != "duplicate" ||
			a[0].Check == nil || a[1].Check != nil || a[2].Check == nil {
			t.Fatalf("Утверждения не сохранили порядок, повторы или функции: %+v", a)
		}
		if !a[0].Check(grammar.AssertionInput{}) || a[2].Check(grammar.AssertionInput{}) {
			t.Fatal("Зарегистрированные проверки вернули неверные ответы")
		}
		wantCalls := []string{
			"detection first", "detection second", "detection first",
			"orchestration first", "orchestration second", "orchestration first",
			"line type first", "line type second", "line type first",
			"line parser first", "line parser second", "line parser first",
			"assertion first", "assertion second",
		}
		if !slices.Equal(calls, wantCalls) {
			t.Fatalf("Порядок вызовов = %v, требуется %v", calls, wantCalls)
		}
		clear(a)
	}
}

func registryTestActions[F ~func(grammar.GrammarContext) bool](stage string, calls *[]string) []F {
	first := F(func(grammar.GrammarContext) bool {
		*calls = append(*calls, stage+" first")
		return true
	})
	second := F(func(grammar.GrammarContext) bool {
		*calls = append(*calls, stage+" second")
		return false
	})
	return []F{first, nil, second, first}
}

func assertRegistryActions[F ~func(grammar.GrammarContext) bool](t *testing.T, functions []F) {
	t.Helper()
	if len(functions) != 4 || functions[0] == nil || functions[1] != nil || functions[2] == nil || functions[3] == nil {
		t.Fatal("Набор функций изменён после регистрации")
	}
	if !functions[0](nil) || functions[2](nil) || !functions[3](nil) {
		t.Fatal("Функции не сохранили порядок и ответы")
	}
	clear(functions)
}

func TestRegistryReplacesWholeCollectionAndPreservesOtherVersions(t *testing.T) {
	for _, mode := range []string{"populated", "nil slices", "empty slices"} {
		t.Run(mode, func(t *testing.T) {
			r := grammar.NewRegistry()
			oldAction := func(grammar.GrammarContext) bool { return false }
			oldAssertion := grammar.Assertion{ID: "old", Check: func(grammar.AssertionInput) bool { return false }}
			for _, version := range []string{"1.2", "other"} {
				r.Register(version, []grammar.DetectionFunc{oldAction}, []grammar.OrchestrationFunc{oldAction},
					[]grammar.LineTypeFunc{oldAction}, []grammar.LineParserFunc{oldAction}, []grammar.Assertion{oldAssertion})
			}
			var detection []grammar.DetectionFunc
			var orchestration []grammar.OrchestrationFunc
			var lineType []grammar.LineTypeFunc
			var lineParser []grammar.LineParserFunc
			var assertions []grammar.Assertion
			switch mode {
			case "populated":
				action := func(grammar.GrammarContext) bool { return true }
				detection, orchestration = []grammar.DetectionFunc{action}, []grammar.OrchestrationFunc{action}
				lineType, lineParser = []grammar.LineTypeFunc{action}, []grammar.LineParserFunc{action}
				assertions = []grammar.Assertion{{ID: "new", Check: func(grammar.AssertionInput) bool { return true }}}
			case "empty slices":
				detection, orchestration = []grammar.DetectionFunc{}, []grammar.OrchestrationFunc{}
				lineType, lineParser, assertions = []grammar.LineTypeFunc{}, []grammar.LineParserFunc{}, []grammar.Assertion{}
			}
			r.Register("1.2", detection, orchestration, lineType, lineParser, assertions)
			g, found := r.Lookup("1.2")
			if !found || g.Version() != "1.2" {
				t.Fatal("Повторная регистрация потеряла версию")
			}
			wantLength := 0
			if mode == "populated" {
				wantLength = 1
			}
			d, o, lt, lp, a := g.DetectionFuncs(), g.OrchestrationFuncs(), g.LineTypeFuncs(), g.LineParserFuncs(), g.Assertions()
			if len(d) != wantLength || len(o) != wantLength || len(lt) != wantLength || len(lp) != wantLength || len(a) != wantLength {
				t.Fatal("Повторная регистрация не заменила все пять наборов полностью")
			}
			if wantLength == 1 {
				if d[0] == nil || o[0] == nil || lt[0] == nil || lp[0] == nil || a[0].Check == nil || a[0].ID != "new" {
					t.Fatal("Новые функции или утверждение потеряны")
				}
				if !d[0](nil) || !o[0](nil) || !lt[0](nil) || !lp[0](nil) || !a[0].Check(grammar.AssertionInput{}) {
					t.Fatal("После замены выполняются прежние функции")
				}
			}
			other, found := r.Lookup("other")
			if !found || other.Version() != "other" {
				t.Fatal("Замена затронула другую версию")
			}
			d, o, lt, lp, a = other.DetectionFuncs(), other.OrchestrationFuncs(), other.LineTypeFuncs(), other.LineParserFuncs(), other.Assertions()
			if len(d) != 1 || len(o) != 1 || len(lt) != 1 || len(lp) != 1 || len(a) != 1 {
				t.Fatal("Изменились наборы другой версии")
			}
			if d[0] == nil || o[0] == nil || lt[0] == nil || lp[0] == nil || a[0].Check == nil || a[0].ID != "old" {
				t.Fatal("Изменились функции другой версии")
			}
			if d[0](nil) || o[0](nil) || lt[0](nil) || lp[0](nil) || a[0].Check(grammar.AssertionInput{}) {
				t.Fatal("Для другой версии выполняются новые функции")
			}
		})
	}
}
