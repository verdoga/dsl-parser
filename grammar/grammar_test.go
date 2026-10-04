package grammar

import (
	"slices"
	"testing"
)

func TestGrammarCollectionVersionPreservesExactValue(t *testing.T) {
	for _, version := range []string{"", "1.2", " 1.2 ", "DSL-next"} {
		t.Run(version, func(t *testing.T) {
			g := grammarCollection{version: version}
			if got := g.Version(); got != version {
				t.Errorf("Version() = %q, требуется %q", got, version)
			}
		})
	}
}

func TestGrammarCollectionEmptyCollections(t *testing.T) {
	tests := []struct {
		name    string
		grammar grammarCollection
	}{
		{name: "zero value"},
		{name: "empty slices", grammar: grammarCollection{
			detectionFuncs:     []DetectionFunc{},
			orchestrationFuncs: []OrchestrationFunc{},
			lineTypeFuncs:      []LineTypeFunc{},
			lineParserFuncs:    []LineParserFunc{},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lengths := []struct {
				name   string
				length int
			}{
				{"DetectionFuncs", len(test.grammar.DetectionFuncs())},
				{"OrchestrationFuncs", len(test.grammar.OrchestrationFuncs())},
				{"LineTypeFuncs", len(test.grammar.LineTypeFuncs())},
				{"LineParserFuncs", len(test.grammar.LineParserFuncs())},
			}
			for _, result := range lengths {
				if result.length != 0 {
					t.Errorf("%s() содержит %d элементов, требуется 0", result.name, result.length)
				}
			}
		})
	}
}

func TestGrammarCollectionFunctionSlicesPreserveOrderAndAreIndependent(t *testing.T) {
	t.Run("detection", func(t *testing.T) {
		assertGrammarFunctionSlice(t, func(functions []DetectionFunc) func() []DetectionFunc {
			g := grammarCollection{detectionFuncs: functions}
			return g.DetectionFuncs
		})
	})
	t.Run("orchestration", func(t *testing.T) {
		assertGrammarFunctionSlice(t, func(functions []OrchestrationFunc) func() []OrchestrationFunc {
			g := grammarCollection{orchestrationFuncs: functions}
			return g.OrchestrationFuncs
		})
	})
	t.Run("line type", func(t *testing.T) {
		assertGrammarFunctionSlice(t, func(functions []LineTypeFunc) func() []LineTypeFunc {
			g := grammarCollection{lineTypeFuncs: functions}
			return g.LineTypeFuncs
		})
	})
	t.Run("line parser", func(t *testing.T) {
		assertGrammarFunctionSlice(t, func(functions []LineParserFunc) func() []LineParserFunc {
			g := grammarCollection{lineParserFuncs: functions}
			return g.LineParserFuncs
		})
	})
}

func assertGrammarFunctionSlice[F ~func(GrammarContext) bool](t *testing.T, collection func([]F) func() []F) {
	t.Helper()
	var calls []string
	first := F(func(GrammarContext) bool {
		calls = append(calls, "first")
		return true
	})
	second := F(func(GrammarContext) bool {
		calls = append(calls, "second")
		return false
	})
	get := collection([]F{first, nil, second, first})
	returned := get()
	if len(calls) != 0 {
		t.Fatal("Получение набора вызвало функции грамматики")
	}
	if len(returned) != 4 {
		t.Fatalf("Получено %d функций, требуется 4", len(returned))
	}
	returned[0] = nil
	returned[1] = second
	returned[2] = first
	returned[3] = second

	for attempt := 1; attempt <= 2; attempt++ {
		calls = nil
		fresh := get()
		if len(calls) != 0 {
			t.Fatal("Повторное получение набора вызвало функции грамматики")
		}
		if len(fresh) != 4 {
			t.Fatalf("Получено %d функций, требуется 4", len(fresh))
		}
		for i, want := range []bool{true, false, false, true} {
			if i == 1 {
				if fresh[i] != nil {
					t.Fatal("Отсутствующая функция заменена")
				}
				continue
			}
			if fresh[i] == nil {
				t.Fatalf("Функция %d потеряна", i)
			}
			if got := fresh[i](nil); got != want {
				t.Errorf("Функция %d вернула %t, требуется %t", i, got, want)
			}
		}
		if !slices.Equal(calls, []string{"first", "second", "first"}) {
			t.Errorf("Порядок функций = %v, требуется [first second first]", calls)
		}
		fresh[0] = nil
	}
}

func TestGrammarCollectionAssertionsPreserveRoleIDs(t *testing.T) {
	tests := []struct {
		name          string
		enterParentID AssertionID
		leaveParentID AssertionID
	}{
		{name: "empty roles"},
		{name: "distinct roles", enterParentID: "enter", leaveParentID: "leave"},
		{name: "roles are not sorted", enterParentID: "z-enter", leaveParentID: "a-leave"},
		{name: "empty enter", leaveParentID: "leave"},
		{name: "empty leave", enterParentID: "enter"},
		{name: "same IDs", enterParentID: "same", leaveParentID: "same"},
		{name: "exact spelling", enterParentID: " Enter ", leaveParentID: "LEAVE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := grammarCollection{enterParentID: test.enterParentID, leaveParentID: test.leaveParentID}
			enter, leave := g.Assertions()
			if enter != test.enterParentID || leave != test.leaveParentID {
				t.Fatalf("Assertions() = (%q, %q), требуется (%q, %q)", enter, leave, test.enterParentID, test.leaveParentID)
			}
		})
	}
}
