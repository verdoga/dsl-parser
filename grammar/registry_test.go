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
		r.Register(version, nil, nil, nil, nil, grammar.AssertionID(version), grammar.AssertionID("leave:"+version))
	}
	for _, version := range versions {
		t.Run(version, func(t *testing.T) {
			g, found := r.Lookup(version)
			if !found || g.Version() != version {
				t.Fatalf("Lookup(%q): версия = %q, found = %t", version, g.Version(), found)
			}
			enter, leave := g.Assertions()
			if string(enter) != version || string(leave) != "leave:"+version {
				t.Fatalf("Для версии %q получены чужие роли: (%q, %q)", version, enter, leave)
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

func TestRegistryPreservesExactRoleIDs(t *testing.T) {
	tests := []struct {
		name          string
		enterParentID grammar.AssertionID
		leaveParentID grammar.AssertionID
	}{
		{name: "empty roles"},
		{name: "empty enter", leaveParentID: "leave"},
		{name: "empty leave", enterParentID: "enter"},
		{name: "same IDs", enterParentID: "same", leaveParentID: "same"},
		{name: "exact spelling", enterParentID: " Enter ", leaveParentID: "LEAVE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r := grammar.NewRegistry()
			r.Register("1.2", nil, nil, nil, nil, test.enterParentID, test.leaveParentID)
			g, found := r.Lookup("1.2")
			if !found {
				t.Fatal("Lookup не нашёл зарегистрированную версию")
			}
			enter, leave := g.Assertions()
			if enter != test.enterParentID || leave != test.leaveParentID {
				t.Fatalf("Assertions() = (%q, %q), требуется (%q, %q)", enter, leave, test.enterParentID, test.leaveParentID)
			}
		})
	}
}

func TestRegistryCopiesAllInputSlicesWithoutCallingFunctions(t *testing.T) {
	var calls []string
	detection := registryTestActions[grammar.DetectionFunc]("detection", &calls)
	orchestration := registryTestActions[grammar.OrchestrationFunc]("orchestration", &calls)
	lineType := registryTestActions[grammar.LineTypeFunc]("line type", &calls)
	lineParser := registryTestActions[grammar.LineParserFunc]("line parser", &calls)
	r := grammar.NewRegistry()
	r.Register("1.2", detection, orchestration, lineType, lineParser, "z-enter", "a-leave")
	if len(calls) != 0 {
		t.Fatal("Register вызвал функции")
	}
	clear(detection)
	clear(orchestration)
	clear(lineType)
	clear(lineParser)
	for attempt := 0; attempt < 2; attempt++ {
		calls = nil
		g, found := r.Lookup("1.2")
		if !found || len(calls) != 0 {
			t.Fatal("Lookup не нашёл версию или вызвал функции")
		}
		d, o, lt, lp := g.DetectionFuncs(), g.OrchestrationFuncs(), g.LineTypeFuncs(), g.LineParserFuncs()
		enter, leave := g.Assertions()
		if len(calls) != 0 {
			t.Fatal("Получение наборов вызвало функции")
		}
		assertRegistryActions(t, d)
		assertRegistryActions(t, o)
		assertRegistryActions(t, lt)
		assertRegistryActions(t, lp)
		if enter != "z-enter" || leave != "a-leave" {
			t.Fatalf("Изменены ID структурных ролей: (%q, %q)", enter, leave)
		}
		wantCalls := []string{
			"detection first", "detection second", "detection first",
			"orchestration first", "orchestration second", "orchestration first",
			"line type first", "line type second", "line type first",
			"line parser first", "line parser second", "line parser first",
		}
		if !slices.Equal(calls, wantCalls) {
			t.Fatalf("Порядок вызовов = %v, требуется %v", calls, wantCalls)
		}
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
			for _, version := range []string{"1.2", "other"} {
				r.Register(version, []grammar.DetectionFunc{oldAction}, []grammar.OrchestrationFunc{oldAction},
					[]grammar.LineTypeFunc{oldAction}, []grammar.LineParserFunc{oldAction}, "old-enter", "old-leave")
			}
			var detection []grammar.DetectionFunc
			var orchestration []grammar.OrchestrationFunc
			var lineType []grammar.LineTypeFunc
			var lineParser []grammar.LineParserFunc
			var enterParentID, leaveParentID grammar.AssertionID
			switch mode {
			case "populated":
				action := func(grammar.GrammarContext) bool { return true }
				detection, orchestration = []grammar.DetectionFunc{action}, []grammar.OrchestrationFunc{action}
				lineType, lineParser = []grammar.LineTypeFunc{action}, []grammar.LineParserFunc{action}
				enterParentID, leaveParentID = "new-enter", "new-leave"
			case "empty slices":
				detection, orchestration = []grammar.DetectionFunc{}, []grammar.OrchestrationFunc{}
				lineType, lineParser = []grammar.LineTypeFunc{}, []grammar.LineParserFunc{}
			}
			r.Register("1.2", detection, orchestration, lineType, lineParser, enterParentID, leaveParentID)
			g, found := r.Lookup("1.2")
			if !found || g.Version() != "1.2" {
				t.Fatal("Повторная регистрация потеряла версию")
			}
			wantLength := 0
			if mode == "populated" {
				wantLength = 1
			}
			d, o, lt, lp := g.DetectionFuncs(), g.OrchestrationFuncs(), g.LineTypeFuncs(), g.LineParserFuncs()
			enter, leave := g.Assertions()
			if len(d) != wantLength || len(o) != wantLength || len(lt) != wantLength || len(lp) != wantLength {
				t.Fatal("Повторная регистрация не заменила все четыре набора полностью")
			}
			if enter != enterParentID || leave != leaveParentID {
				t.Fatalf("Повторная регистрация не заменила роли: (%q, %q), требуется (%q, %q)", enter, leave, enterParentID, leaveParentID)
			}
			if wantLength == 1 {
				if d[0] == nil || o[0] == nil || lt[0] == nil || lp[0] == nil {
					t.Fatal("Новые функции потеряны")
				}
				if !d[0](nil) || !o[0](nil) || !lt[0](nil) || !lp[0](nil) {
					t.Fatal("После замены выполняются прежние функции")
				}
			}
			other, found := r.Lookup("other")
			if !found || other.Version() != "other" {
				t.Fatal("Замена затронула другую версию")
			}
			d, o, lt, lp = other.DetectionFuncs(), other.OrchestrationFuncs(), other.LineTypeFuncs(), other.LineParserFuncs()
			enter, leave = other.Assertions()
			if len(d) != 1 || len(o) != 1 || len(lt) != 1 || len(lp) != 1 {
				t.Fatal("Изменились наборы другой версии")
			}
			if d[0] == nil || o[0] == nil || lt[0] == nil || lp[0] == nil {
				t.Fatal("Изменились функции другой версии")
			}
			if enter != "old-enter" || leave != "old-leave" {
				t.Fatalf("Изменились роли другой версии: (%q, %q)", enter, leave)
			}
			if d[0](nil) || o[0](nil) || lt[0](nil) || lp[0](nil) {
				t.Fatal("Для другой версии выполняются новые функции")
			}
		})
	}
}
