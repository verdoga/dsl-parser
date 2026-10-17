package parser

import (
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
)

func TestDiagnosticRulesSelectExactVersionAndCode(t *testing.T) {
	for _, tc := range []struct {
		version string
		code    diagnostics.Code
		called  string
	}{
		{"1.2", diagnostics.P012, "1.2/P012"},
		{"2.0", diagnostics.P012, "2.0/P012"},
		{"1.2", diagnostics.P008, "1.2/P008"},
		{"unknown", diagnostics.P012, ""},
		{"1.2 ", diagnostics.P012, ""},
		{"1.2", "p012", ""},
		{"1.2", "UNKNOWN", ""},
	} {
		t.Run(tc.version+"/"+string(tc.code), func(t *testing.T) {
			var calls []string
			rules := diagnosticRules{}
			for _, key := range []diagnosticRuleKey{{"1.2", diagnostics.P012}, {"2.0", diagnostics.P012}, {"1.2", diagnostics.P008}} {
				rules[key] = []diagnosticRule{func(input diagnosticRuleInput) (diagnosticDecision, error) {
					if input.Version != key.Version || input.Occurrence.DiagnosticCode != key.Code {
						t.Fatal("Правило получило чужую версию или код")
					}
					calls = append(calls, key.Version+"/"+string(key.Code))
					return diagnosticSuppress, nil
				}}
			}
			input := diagnosticRuleInput{Version: tc.version, Occurrence: model.Diagnostic{DiagnosticCode: tc.code}}
			keep, err := rules.accept(input)
			if err != nil || keep != (tc.called == "") {
				t.Fatalf("accept() = %t, %v", keep, err)
			}
			var want []string
			if tc.called != "" {
				want = []string{tc.called}
			}
			if !slices.Equal(calls, want) {
				t.Fatalf("Вызваны %v, требуется %v", calls, want)
			}
		})
	}
}

func TestDiagnosticRulesOrderAndFinalDecisions(t *testing.T) {
	for _, tc := range []struct {
		name      string
		decisions []diagnosticDecision
		keep      bool
		calls     int
	}{
		{"empty", nil, true, 0},
		{"continue only", []diagnosticDecision{diagnosticContinue, diagnosticContinue}, true, 2},
		{"keep first", []diagnosticDecision{diagnosticKeep, diagnosticSuppress}, true, 1},
		{"suppress first", []diagnosticDecision{diagnosticSuppress, diagnosticKeep}, false, 1},
		{"continue then keep", []diagnosticDecision{diagnosticContinue, diagnosticKeep, diagnosticSuppress}, true, 2},
		{"continue then suppress", []diagnosticDecision{diagnosticContinue, diagnosticSuppress, diagnosticKeep}, false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := diagnosticRuleKey{"1.2", diagnostics.P012}
			rules := diagnosticRules{}
			var calls []int
			for i, decision := range tc.decisions {
				rules[key] = append(rules[key], func(diagnosticRuleInput) (diagnosticDecision, error) {
					calls = append(calls, i)
					return decision, nil
				})
			}
			keep, err := rules.accept(diagnosticFilterSource())
			if err != nil || keep != tc.keep || len(calls) != tc.calls {
				t.Fatalf("accept() = %t, %v; вызовы %v", keep, err, calls)
			}
			for i, got := range calls {
				if got != i {
					t.Fatalf("Нарушен порядок обработчиков: %v", calls)
				}
			}
		})
	}
}

func TestDiagnosticRulesWithoutHandlersKeepDiagnostic(t *testing.T) {
	key := diagnosticRuleKey{"1.2", diagnostics.P012}
	for _, rules := range []diagnosticRules{nil, {}, {key: nil}, {key: {}}} {
		if keep, err := rules.accept(diagnosticFilterSource()); !keep || err != nil {
			t.Fatalf("Пустой набор изменил диагностику: %t, %v", keep, err)
		}
	}
}

func TestDiagnosticRulesTechnicalErrorsStopChain(t *testing.T) {
	failure := errors.New("сбой проверки")
	for _, tc := range []struct {
		name string
		rule diagnosticRule
		wrap bool
	}{
		{"nil handler", nil, false},
		{"unknown decision", func(diagnosticRuleInput) (diagnosticDecision, error) { return diagnosticDecision(255), nil }, false},
		{"error with continue", func(diagnosticRuleInput) (diagnosticDecision, error) { return diagnosticContinue, failure }, true},
		{"error with keep", func(diagnosticRuleInput) (diagnosticDecision, error) { return diagnosticKeep, failure }, true},
		{"error with suppress", func(diagnosticRuleInput) (diagnosticDecision, error) { return diagnosticSuppress, failure }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := diagnosticRules{{"1.2", diagnostics.P012}: {
				func(diagnosticRuleInput) (diagnosticDecision, error) { return diagnosticContinue, nil },
				tc.rule,
				func(diagnosticRuleInput) (diagnosticDecision, error) {
					t.Fatal("Цепочка продолжилась после технической ошибки")
					return diagnosticKeep, nil
				},
			}}
			keep, err := rules.accept(diagnosticFilterSource())
			if err == nil || keep || (tc.wrap && !errors.Is(err, failure)) {
				t.Fatalf("Техническая ошибка потеряна: %t, %v", keep, err)
			}
		})
	}
}

func TestDiagnosticRulesProtectStructuralDiagnostics(t *testing.T) {
	for _, code := range []diagnostics.Code{diagnostics.P010, diagnostics.P015} {
		for _, decision := range []diagnosticDecision{diagnosticContinue, diagnosticKeep, diagnosticSuppress} {
			input := diagnosticFilterSource()
			input.Occurrence.DiagnosticCode = code
			rules := diagnosticRules{{input.Version, code}: {
				func(got diagnosticRuleInput) (diagnosticDecision, error) {
					// Подмена кода во входной копии не должна обходить запрет.
					got.Occurrence.DiagnosticCode = diagnostics.P012
					return decision, nil
				},
			}}
			keep, err := rules.accept(input)
			if decision == diagnosticSuppress {
				if err == nil || keep {
					t.Fatalf("Удаление %s разрешено: %t, %v", code, keep, err)
				}
			} else if err != nil || !keep {
				t.Fatalf("Сохранение %s запрещено: %t, %v", code, keep, err)
			}
		}
	}
}

func TestDiagnosticRuleInputMatchesFullElementLocation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		location *model.Location
		matched  bool
	}{
		{"exact", &model.Location{Start: model.Position{Line: 5, Column: 2}, End: model.Position{Line: 5, Column: 5}}, true},
		{"no location", nil, false},
		{"other start line", &model.Location{Start: model.Position{Line: 4, Column: 2}, End: model.Position{Line: 5, Column: 5}}, false},
		{"other end line", &model.Location{Start: model.Position{Line: 5, Column: 2}, End: model.Position{Line: 6, Column: 5}}, false},
		{"partial element", &model.Location{Start: model.Position{Line: 5, Column: 3}, End: model.Position{Line: 5, Column: 5}}, false},
		{"wrong end", &model.Location{Start: model.Position{Line: 5, Column: 2}, End: model.Position{Line: 5, Column: 6}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := diagnosticFilterSource()
			source.Occurrence.Location = tc.location
			input := newDiagnosticRuleInput(source.Version, source.Occurrence, source.Line, nil, nil)
			if (input.Element != nil) != tc.matched || input.Parent != nil || input.OpenBlock != nil {
				t.Fatalf("Неверные связи снимка: %+v", input)
			}
			if tc.matched && !reflect.DeepEqual(*input.Element, source.Line.Elements[0]) {
				t.Fatal("Потеряны свойства выбранного элемента")
			}
		})
	}
}

func TestDiagnosticRuleInputSnapshotSurvivesSourceChanges(t *testing.T) {
	source := diagnosticFilterSource()
	input := newDiagnosticRuleInput(source.Version, source.Occurrence, source.Line, source.Parent, source.OpenBlock)
	if !reflect.DeepEqual(input, source) {
		t.Fatal("Снимок потерял исходные свойства")
	}
	want := parserSnapshot(t, input)
	mutateDiagnosticFilterInput(&source)
	if parserSnapshot(t, input) != want {
		t.Fatal("Снимок изменился вместе с исходными данными")
	}
	// Выбранный элемент и два указателя на одну исходную строку тоже отделены.
	input.Element.Raw = "изменённый элемент"
	*input.Element.Value = "изменённое значение"
	input.Element.ErrorIDs[0] = "другой ID"
	if input.Line.Elements[0].Raw != "Ёж🌍" || *input.Line.Elements[0].Value != "Ёж🌍" || input.Line.Elements[0].ErrorIDs[0] != "existing" {
		t.Fatal("Element разделяет изменяемые данные с Line")
	}
	input.Parent.Number = 99
	*input.Parent.ParentLine = 99
	*input.Parent.Elements[0].Value = "changed"
	input.Parent.Elements[0].ErrorIDs[0] = "changed"
	if input.OpenBlock.Number != 4 || *input.OpenBlock.ParentLine != 1 || *input.OpenBlock.Elements[0].Value != "multifill" || input.OpenBlock.Elements[0].ErrorIDs[0] != "parent-error" {
		t.Fatal("Parent разделяет изменяемые данные с OpenBlock")
	}
}

func TestDiagnosticRuleInputDistinguishesParentAndOpenBlock(t *testing.T) {
	parent := model.Line{Number: 2, Raw: "@task A", LineType: model.LineTypeTag}
	block := model.Line{Number: 3, Raw: "@multifill {", LineType: model.LineTypeBlockStart}
	for _, tc := range []struct {
		name   string
		parent *model.Line
		block  *model.Line
	}{
		{"neither", nil, nil},
		{"parent only", &parent, nil},
		{"block only", nil, &block},
		{"both", &parent, &block},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := diagnosticFilterSource()
			input := newDiagnosticRuleInput(source.Version, source.Occurrence, source.Line, tc.parent, tc.block)
			if !reflect.DeepEqual(input.Parent, tc.parent) || !reflect.DeepEqual(input.OpenBlock, tc.block) {
				t.Fatal("Логический родитель и открытый блок перепутаны или утрачены")
			}
		})
	}
}

func TestDiagnosticRulesIsolateSourceAndSuccessiveHandlers(t *testing.T) {
	input := diagnosticFilterSource()
	before := parserSnapshot(t, input)
	var retained diagnosticRuleInput
	calls := 0
	rules := diagnosticRules{{input.Version, input.Occurrence.DiagnosticCode}: {
		func(got diagnosticRuleInput) (diagnosticDecision, error) {
			calls++
			if parserSnapshot(t, got) != before {
				t.Fatal("Первое правило получило изменённые данные")
			}
			retained = got
			mutateDiagnosticFilterInput(&got)
			return diagnosticContinue, nil
		},
		func(got diagnosticRuleInput) (diagnosticDecision, error) {
			calls++
			if parserSnapshot(t, got) != before {
				t.Fatal("Второе правило увидело изменения первого")
			}
			return diagnosticKeep, nil
		},
	}}
	keep, err := rules.accept(input)
	if err != nil || !keep || calls != 2 {
		t.Fatalf("Цепочка не завершена: %t, %v; вызовов %d", keep, err, calls)
	}
	mutateDiagnosticFilterInput(&retained)
	if parserSnapshot(t, input) != before {
		t.Fatal("Правило изменило данные вызывающего кода")
	}
}

func TestDiagnosticRuleSnapshotsPreserveAbsentAndEmptyValues(t *testing.T) {
	for _, input := range []diagnosticRuleInput{
		{},
		{Occurrence: model.Diagnostic{RelatedLocations: []model.Location{}}, Line: model.Line{Elements: []model.Element{}}},
		{Line: model.Line{Elements: []model.Element{{ErrorIDs: []string{}}}}, Element: &model.Element{}, Parent: &model.Line{}, OpenBlock: &model.Line{}},
	} {
		if got := cloneDiagnosticRuleInput(input); !reflect.DeepEqual(input, got) {
			t.Fatalf("Копирование изменило nil или пустые значения: %+v", got)
		}
	}
}

func TestBuiltinDiagnosticRuleSetsAreIndependent(t *testing.T) {
	first, second := builtinDiagnosticRules(), builtinDiagnosticRules()
	key := diagnosticRuleKey{"test", "TEST"}
	first[key] = []diagnosticRule{func(diagnosticRuleInput) (diagnosticDecision, error) { return diagnosticSuppress, nil }}
	if _, found := second[key]; found {
		t.Fatal("Фабрика вернула общую изменяемую карту")
	}
	input := diagnosticRuleInput{Version: key.Version, Occurrence: model.Diagnostic{DiagnosticCode: key.Code}}
	if keep, err := second.accept(input); !keep || err != nil {
		t.Fatalf("Изменение первого набора повлияло на второй: %t, %v", keep, err)
	}
}

func diagnosticFilterSource() diagnosticRuleInput {
	parentNumber, ancestorNumber := 4, 1
	value, tag := "Ёж🌍", "multifill"
	parent := model.Line{Number: 4, ParentLine: &ancestorNumber, LineType: model.LineTypeBlockStart,
		Elements: []model.Element{{ElementType: model.ElementTypeTag, Value: &tag, ErrorIDs: []string{"parent-error"}}}}
	line := model.Line{Number: 5, ParentLine: &parentNumber, LineType: model.LineTypeContent, Raw: " Ёж🌍 ",
		Elements: []model.Element{{ElementType: model.ElementTypeContent, Raw: value, Value: &value, Start: 2, End: 5, ErrorIDs: []string{"existing"}}}}
	return diagnosticRuleInput{
		Version: "1.2",
		Occurrence: model.Diagnostic{ID: "existing", Source: "old", DiagnosticCode: diagnostics.P012,
			Location:         &model.Location{Start: model.Position{Line: 5, Column: 2}, End: model.Position{Line: 5, Column: 5}},
			RelatedLocations: []model.Location{{Start: model.Position{Line: 1, Column: 1}, End: model.Position{Line: 1, Column: 2}}}},
		Line: line, Element: &line.Elements[0], Parent: &parent, OpenBlock: &parent,
	}
}

func mutateDiagnosticFilterInput(input *diagnosticRuleInput) {
	input.Version = "changed"
	input.Occurrence.DiagnosticCode = diagnostics.P003
	input.Occurrence.Location.Start.Column = 99
	input.Occurrence.RelatedLocations[0].End.Column = 99
	input.Element.Raw = "changed"
	*input.Element.Value = "changed"
	input.Element.ErrorIDs[0] = "changed"
	for _, line := range []*model.Line{&input.Line, input.Parent, input.OpenBlock} {
		line.Number = 99
		*line.ParentLine = 99
		line.Elements[0].Raw = "changed"
		*line.Elements[0].Value = "changed"
		line.Elements[0].ErrorIDs[0] = "changed"
	}
}
