package parser

import (
	"errors"
	"slices"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestCompleteDiagnosticNormsAndConcreteDetails(t *testing.T) {
	p := New(nil, nil, nil, diagnostics.NewRegistry(), nil)
	for _, tc := range []struct {
		code  diagnostics.Code
		scope diagnostics.Scope
		fatal bool
	}{
		{diagnostics.IO001, diagnostics.ScopeDocument, true},
		{diagnostics.P001, diagnostics.ScopeDocument, true}, {diagnostics.P002, diagnostics.ScopeDocument, true},
		{diagnostics.P003, diagnostics.ScopeElement, false}, {diagnostics.P004, diagnostics.ScopeLine, false},
		{diagnostics.P005, diagnostics.ScopeLine, false}, {diagnostics.P006, diagnostics.ScopeElement, false},
		{diagnostics.P007, diagnostics.ScopeLine, false}, {diagnostics.P008, diagnostics.ScopeLine, false},
		{diagnostics.P009, diagnostics.ScopeLine, false}, {diagnostics.P010, diagnostics.ScopeLine, false},
		{diagnostics.P011, diagnostics.ScopeBlock, false}, {diagnostics.P012, diagnostics.ScopeElement, false},
		{diagnostics.P013, diagnostics.ScopeDocument, true}, {diagnostics.P014, diagnostics.ScopeDocument, true},
		{diagnostics.P015, diagnostics.ScopeElement, false},
	} {
		t.Run(string(tc.code), func(t *testing.T) {
			occurrence := lineDiagnostic(t, tc.code, 3, 2, 4)
			occurrence.DiagnosticScope, occurrence.SeverityLevel = diagnostics.ScopeBlock, diagnostics.SeverityWarning
			occurrence.Message, occurrence.Fatal = "Конкретное описание", !tc.fatal
			occurrence.RelatedLocations = []model.Location{*lineDiagnostic(t, tc.code, 1, 1, 2).Location}
			before := parserSnapshot(t, occurrence)
			got, err := p.completeDiagnostic(occurrence)
			if err != nil || got.SeverityLevel != diagnostics.SeverityError || got.DiagnosticScope != tc.scope || got.Fatal != tc.fatal {
				t.Fatalf("Нормативные свойства: %+v, %v", got, err)
			}
			if got.Message != occurrence.Message || got.Location != occurrence.Location || !slices.Equal(got.RelatedLocations, occurrence.RelatedLocations) || parserSnapshot(t, occurrence) != before {
				t.Fatal("Конкретные сведения или исходный случай изменены")
			}
			got, err = p.completeDiagnostic(model.Diagnostic{DiagnosticCode: tc.code})
			description, _ := p.diagnostics.Lookup(tc.code)
			if err != nil || got.Message != description.Message() || got.Location != nil || got.RelatedLocations == nil {
				t.Fatalf("Не дополнен пустой случай: %+v, %v", got, err)
			}
			if tc.code == diagnostics.P015 && got.Message != "Незавершённый разбор строки" {
				t.Fatal("Неверное сообщение P015")
			}
		})
	}
	for _, registry := range []diagnostics.Registry{nil, diagnostics.NewRegistry()} {
		p.diagnostics = registry
		if _, err := p.completeDiagnostic(model.Diagnostic{DiagnosticCode: "P016"}); err == nil {
			t.Fatal("Отсутствующий реестр или код принят")
		}
	}
}

func TestAppendDiagnosticSkipsOccupiedIDsAndKeepsHistory(t *testing.T) {
	result := parserInitialResult(t)
	result.Diagnostics = []model.Diagnostic{{ID: "d1", Source: "old"}, {ID: "d3", Source: "old"}, {ID: "external", Source: "old"}}
	before := parserSnapshot(t, result.Diagnostics)
	p := New(nil, result, nil, nil, nil)
	p.processingID = "current"
	seen := map[string]bool{"d1": true, "d3": true, "external": true}
	for range 4 {
		id := p.appendDiagnostic(model.Diagnostic{ID: "external", Source: "wrong", Message: "Сохранить"})
		got := result.Diagnostics[len(result.Diagnostics)-1]
		if id == "" || seen[id] || got.ID != id || got.Source != "current" || got.Message != "Сохранить" {
			t.Fatalf("Неверный ID, источник или содержимое: %+v", got)
		}
		seen[id] = true
	}
	if parserSnapshot(t, result.Diagnostics[:3]) != before {
		t.Fatal("Прежние записи изменены")
	}
}

func TestImportLineDiagnosticsDeduplicatesAndLinks(t *testing.T) {
	result := parserInitialResult(t)
	old := lineDiagnostic(t, diagnostics.P003, 1, 1, 2)
	old.ID, old.Source = "historic", "old"
	result.Diagnostics = []model.Diagnostic{old}
	p := New(nil, result, nil, diagnostics.NewRegistry(), nil)
	p.processingID = "current"
	line := model.Line{Number: 1, Raw: "аб", Elements: []model.Element{
		{Raw: "а", Start: 1, End: 2, ErrorIDs: []string{"historic"}}, {Raw: "б", Start: 2, End: 3},
	}}
	context := grammar.NewContext(line.Raw)
	first := lineDiagnostic(t, diagnostics.P003, 1, 2, 3)
	first.Message = "Первое обнаружение"
	context.AddDiagnostic(first)
	duplicate := first
	duplicate.Location = lineDiagnostic(t, diagnostics.P003, 1, 2, 3).Location
	duplicate.Message = "Повтор"
	context.AddDiagnostic(duplicate)
	context.AddDiagnostic(lineDiagnostic(t, diagnostics.P003, 1, 1, 2))
	context.AddDiagnostic(lineDiagnostic(t, diagnostics.P015, 1, 1, 2))
	context.AddDiagnostic(lineDiagnostic(t, diagnostics.P004, 1, 1, 2))
	for range 2 {
		if err := p.importLineDiagnostics(context, &line); err != nil {
			t.Fatal(err)
		}
	}
	if len(result.Diagnostics) != 5 || result.Diagnostics[1].Message != first.Message || parserSnapshot(t, result.Diagnostics[0]) != parserSnapshot(t, old) {
		t.Fatalf("Повторы или история обработаны неверно: %+v", result.Diagnostics)
	}
	if len(line.Elements[0].ErrorIDs) != 3 || len(line.Elements[1].ErrorIDs) != 1 {
		t.Fatalf("Неверные ссылки элементов: %+v", line.Elements)
	}
	for _, diagnostic := range result.Diagnostics[1:] {
		if diagnostic.Source != "current" {
			t.Fatal("Неверный источник новой диагностики")
		}
	}
	result.Lines = []model.Line{line}
	if err := checkElementLinks(result); err != nil {
		t.Fatal(err)
	}
	for _, code := range []diagnostics.Code{diagnostics.P015, "P016"} {
		context = grammar.NewContext("")
		context.AddDiagnostic(lineDiagnostic(t, code, 2, 1, 1))
		if err := p.importLineDiagnostics(context, &model.Line{Number: 2}); err == nil {
			t.Fatalf("Принят отсутствующий элемент или неизвестный код %s", code)
		}
	}
}

func TestImportP006NormalizesScopeAndLinksExactRange(t *testing.T) {
	for _, tc := range []struct {
		name, raw  string
		start, end int
	}{
		{"present Unicode", "Ёж🌍", 2, 5},
		{"missing", "", 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, scope := range []diagnostics.Scope{diagnostics.ScopeElement, diagnostics.ScopeLine} {
				p := New(nil, parserInitialResult(t), nil, diagnostics.NewRegistry(), nil)
				p.processingID, p.version = "current", "1.2"
				line := model.Line{Number: 1, Raw: " " + tc.raw, Elements: []model.Element{
					{ElementType: model.ElementTypeUnparsed, Raw: tc.raw, Value: nil, Start: tc.start, End: tc.end},
				}}
				context := grammar.NewContext(line.Raw)
				occurrence := lineDiagnostic(t, diagnostics.P006, 1, tc.start, tc.end)
				occurrence.ID, occurrence.Source = "foreign-id", "foreign-source"
				occurrence.DiagnosticScope, occurrence.SeverityLevel, occurrence.Fatal = scope, diagnostics.SeverityWarning, true
				context.AddDiagnostic(occurrence)
				before := parserSnapshot(t, context.Diagnostics())
				for range 2 {
					if err := p.importLineDiagnostics(context, &line); err != nil {
						t.Fatal(err)
					}
				}
				if len(p.result.Diagnostics) != 1 || parserSnapshot(t, context.Diagnostics()) != before {
					t.Fatalf("P006 продублирована или изменён исходный случай: %+v", p.result.Diagnostics)
				}
				d := p.result.Diagnostics[0]
				if d.ID == "" || d.ID == occurrence.ID || d.Source != "current" || d.DiagnosticCode != diagnostics.P006 || d.DiagnosticScope != diagnostics.ScopeElement || d.SeverityLevel != diagnostics.SeverityError || d.Fatal || d.Location == nil || *d.Location != *occurrence.Location {
					t.Fatalf("P006 неверно нормализована или перенесена: %+v", d)
				}
				if !slices.Equal(line.Elements[0].ErrorIDs, []string{d.ID}) {
					t.Fatalf("P006 не связана с unparsed ровно один раз: %+v", line.Elements)
				}
				p.result.Lines = []model.Line{line}
				if err := checkElementLinks(p.result); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestP006InParsedResultPreservesOperandRangesAndLinks(t *testing.T) {
	type operandRange struct {
		raw        string
		start, end int
	}
	for _, tc := range []struct {
		source   string
		operands []operandRange
	}{
		{`@media vidio "Unit 07"`, []operandRange{{"vidio", 8, 13}}},
		{" \t@media Медиа \"Ёж🌍\"", []operandRange{{"Медиа", 10, 15}}},
		{"@media", []operandRange{{"", 7, 7}}},
		{"@media audio", []operandRange{{"", 13, 13}}},
		{"@media vidio", []operandRange{{"vidio", 8, 13}, {"", 13, 13}}},
		{"@order", []operandRange{{"", 7, 7}}},
		{"@order １２", []operandRange{{"１２", 8, 10}}},
	} {
		t.Run(tc.source, func(t *testing.T) {
			p := inputParser(t, "@dsl-version 1.2\n"+tc.source+"\nText")
			if err := p.Parse(); err != nil {
				t.Fatal(err)
			}
			result := p.result
			if len(result.Lines) != 3 || len(result.Diagnostics) != len(tc.operands) || !result.Document.HasErrors || !result.Lines[1].HasErrors || result.Lines[0].HasErrors || result.Lines[2].HasErrors {
				t.Fatalf("Неверные диагностики или признаки ошибок: %+v", result)
			}
			var unparsed []model.Element
			for _, element := range result.Lines[1].Elements {
				if element.ElementType == model.ElementTypeUnparsed {
					unparsed = append(unparsed, element)
				} else if len(element.ErrorIDs) != 0 {
					t.Fatalf("P006 привязана к корректному соседнему элементу: %+v", element)
				}
			}
			if len(unparsed) != len(tc.operands) {
				t.Fatalf("Неверное число unparsed: %+v", unparsed)
			}
			ids := make(map[string]bool)
			for i, want := range tc.operands {
				element, d := unparsed[i], result.Diagnostics[i]
				location := model.Location{Start: model.Position{Line: 2, Column: want.start}, End: model.Position{Line: 2, Column: want.end}}
				if element.Raw != want.raw || element.Value != nil || element.Start != want.start || element.End != want.end || !slices.Equal(element.ErrorIDs, []string{d.ID}) {
					t.Fatalf("Неверный unparsed или его ссылка: %+v", element)
				}
				if d.ID == "" || ids[d.ID] || d.Source != "current" || d.DiagnosticCode != diagnostics.P006 || d.DiagnosticScope != diagnostics.ScopeElement || d.SeverityLevel != diagnostics.SeverityError || d.Fatal || d.Location == nil || *d.Location != location || d.Message != "Отсутствует или недопустим обязательный структурный параметр" {
					t.Fatalf("Неверная итоговая P006: %+v, требуется диапазон %+v", d, location)
				}
				ids[d.ID] = true
			}
		})
	}
}

func TestImportP008ElementExceptionOnlyForStandaloneBrace(t *testing.T) {
	for _, raw := range []string{"{", " \t{ \t", "@editor {", "{{"} {
		t.Run(raw, func(t *testing.T) {
			result := parserInitialResult(t)
			p := New(nil, result, nil, diagnostics.NewRegistry(), nil)
			p.processingID = "current"
			p.version = "1.2"
			calls := 0
			p.diagnosticRules[diagnosticRuleKey{"1.2", diagnostics.P008}] = []diagnosticRule{
				func(input diagnosticRuleInput) (diagnosticDecision, error) {
					calls++
					if input.Occurrence.DiagnosticScope != diagnostics.ScopeElement {
						t.Fatal("P008 скорректирована до вызова фильтра")
					}
					return diagnosticKeep, nil
				},
			}
			start := 1
			if raw == " \t{ \t" {
				start = 3
			}
			line := model.Line{Number: 1, Raw: raw, Elements: []model.Element{{Raw: "{", Start: start, End: start + 1}}}
			context := grammar.NewContext(raw)
			context.AddDiagnostic(lineDiagnostic(t, diagnostics.P008, 1, start, start+1))
			before := parserSnapshot(t, context.Diagnostics())
			if err := p.importLineDiagnostics(context, &line); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || parserSnapshot(t, context.Diagnostics()) != before {
				t.Fatal("Фильтр не вызван либо изменена исходная P008")
			}
			wantScope, wantLinks := diagnostics.ScopeLine, 0
			if raw == "{" || raw == " \t{ \t" {
				wantScope, wantLinks = diagnostics.ScopeElement, 1
			}
			if result.Diagnostics[0].DiagnosticScope != wantScope || result.Diagnostics[0].Fatal || len(line.Elements[0].ErrorIDs) != wantLinks {
				t.Fatalf("Неверное исключение P008: %+v, %+v", result.Diagnostics, line.Elements)
			}
		})
	}
}

func TestImportDiagnosticFilterDefaultsPreserveResult(t *testing.T) {
	for _, mode := range []string{"nil", "empty", "other version", "other code", "continue", "keep"} {
		t.Run(mode, func(t *testing.T) {
			p := New(nil, parserInitialResult(t), nil, diagnostics.NewRegistry(), nil)
			p.version, p.processingID = "1.2", "current"
			p.diagnosticRules = diagnosticRules{}
			key := diagnosticRuleKey{"1.2", diagnostics.P012}
			calls := 0
			rule := func(diagnosticRuleInput) (diagnosticDecision, error) {
				calls++
				if mode == "keep" {
					return diagnosticKeep, nil
				}
				return diagnosticContinue, nil
			}
			switch mode {
			case "nil":
				p.diagnosticRules = nil
			case "other version":
				p.diagnosticRules[diagnosticRuleKey{"2.0", key.Code}] = []diagnosticRule{rule}
			case "other code":
				p.diagnosticRules[diagnosticRuleKey{key.Version, diagnostics.P008}] = []diagnosticRule{rule}
			case "continue", "keep":
				p.diagnosticRules[key] = []diagnosticRule{rule}
			}
			if mode != "continue" && mode != "keep" {
				// Без подходящего правила состояние для снимка не читается.
				p.openBlocks = []int{99}
			}
			line := model.Line{Number: 1, Raw: "{x}", LineType: model.LineTypeContent,
				Elements: []model.Element{{ElementType: model.ElementTypeContent, Raw: "{x}", Start: 1, End: 4}}}
			context := grammar.NewContext(line.Raw)
			context.AddDiagnostic(lineDiagnostic(t, diagnostics.P012, 1, 1, 4))
			if err := p.importLineDiagnostics(context, &line); err != nil {
				t.Fatal(err)
			}
			want := lineDiagnostic(t, diagnostics.P012, 1, 1, 4)
			want.ID, want.Source = "d1", "current"
			want.Message = "Неэкранированная фигурная скобка в текстовом значении"
			want.RelatedLocations = []model.Location{}
			if parserSnapshot(t, p.result.Diagnostics) != parserSnapshot(t, []model.Diagnostic{want}) ||
				!slices.Equal(line.Elements[0].ErrorIDs, []string{"d1"}) || p.nextDiagnosticID != 2 {
				t.Fatalf("Изменён прежний результат импорта: %+v; %+v", p.result.Diagnostics, line)
			}
			wantCalls := 0
			if mode == "continue" || mode == "keep" {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("Вызовов фильтра %d, требуется %d", calls, wantCalls)
			}
		})
	}
}

func TestImportDiagnosticFilterSuppressesBeforeIDsWithoutMutatingContext(t *testing.T) {
	p := New(nil, parserInitialResult(t), nil, diagnostics.NewRegistry(), nil)
	p.version, p.processingID = "1.2", "current"
	blockNumber, parentNumber := 1, 2
	p.result.Lines = []model.Line{
		{Number: 1, Raw: "@variants {", LineType: model.LineTypeBlockStart},
		{Number: 2, Raw: "@task A", LineType: model.LineTypeTag, ParentLine: &blockNumber},
	}
	p.openBlocks = []int{blockNumber}
	value := "{x}"
	line := model.Line{Number: 3, Raw: value, ParentLine: &parentNumber, LineType: model.LineTypeContent,
		Elements: []model.Element{{ElementType: model.ElementTypeContent, Raw: value, Value: &value, Start: 1, End: 4}}}
	context := grammar.NewContext(line.Raw)
	context.SetLine(line)
	occurrence := lineDiagnostic(t, diagnostics.P012, 3, 1, 4)
	occurrence.RelatedLocations = []model.Location{*lineDiagnostic(t, diagnostics.P012, 2, 1, 2).Location}
	context.AddDiagnostic(occurrence)
	beforeResult, beforeLine := parserSnapshot(t, p.result), parserSnapshot(t, line)
	beforeContext, beforeDiagnostics := parserSnapshot(t, context.Line()), parserSnapshot(t, context.Diagnostics())
	p.diagnosticRules[diagnosticRuleKey{"1.2", diagnostics.P012}] = []diagnosticRule{
		func(input diagnosticRuleInput) (diagnosticDecision, error) {
			if input.Parent == nil || input.Parent.Number != 2 || input.OpenBlock == nil || input.OpenBlock.Number != 1 ||
				input.Element == nil || input.Element.Raw != value || input.Line.ParentLine == nil || *input.Line.ParentLine != 2 {
				t.Fatalf("Неверный снимок контекста: %+v", input)
			}
			input.Occurrence.Location.Start.Column = 99
			input.Occurrence.RelatedLocations[0].Start.Line = 99
			*input.Line.ParentLine = 99
			*input.Line.Elements[0].Value = "changed"
			input.Element.ErrorIDs = append(input.Element.ErrorIDs, "changed")
			*input.Parent.ParentLine = 99
			input.OpenBlock.Raw = "changed"
			return diagnosticSuppress, nil
		},
	}
	if err := p.importLineDiagnostics(context, &line); err != nil {
		t.Fatal(err)
	}
	if p.nextDiagnosticID != 1 || parserSnapshot(t, p.result) != beforeResult || parserSnapshot(t, line) != beforeLine ||
		parserSnapshot(t, context.Line()) != beforeContext || parserSnapshot(t, context.Diagnostics()) != beforeDiagnostics ||
		!slices.Equal(p.openBlocks, []int{1}) {
		t.Fatal("Подавленная диагностика изменила ID, ссылки, контекст или состояние парсера")
	}
	context.AddDiagnostic(lineDiagnostic(t, diagnostics.P003, 3, 1, 4))
	if err := p.importLineDiagnostics(context, &line); err != nil {
		t.Fatal(err)
	}
	if len(p.result.Diagnostics) != 1 || p.result.Diagnostics[0].ID != "d1" || p.result.Diagnostics[0].DiagnosticCode != diagnostics.P003 ||
		!slices.Equal(line.Elements[0].ErrorIDs, []string{"d1"}) {
		t.Fatal("Подавление повлияло на регистрацию следующей диагностики")
	}
}

func TestParserInstancesHaveIndependentDiagnosticRules(t *testing.T) {
	first := inputParser(t, "@dsl-version 1.2\nText {x}")
	second := inputParser(t, "@dsl-version 1.2\nText {x}")
	key := diagnosticRuleKey{"1.2", diagnostics.P012}
	secondRuleCount := len(second.diagnosticRules[key])
	first.diagnosticRules[key] = []diagnosticRule{func(diagnosticRuleInput) (diagnosticDecision, error) { return diagnosticKeep, nil }}
	first.diagnosticRules[key][0] = func(diagnosticRuleInput) (diagnosticDecision, error) { return diagnosticSuppress, nil }
	if len(second.diagnosticRules[key]) != secondRuleCount {
		t.Fatal("Экземпляры разделяют набор правил")
	}
	for _, p := range []*Parser{first, second} {
		if err := p.Parse(); err != nil {
			t.Fatal(err)
		}
	}
	if len(first.result.Diagnostics) != 0 || first.result.Document.HasErrors || len(second.result.Diagnostics) != 1 ||
		second.result.Diagnostics[0].DiagnosticCode != diagnostics.P012 || !second.result.Document.HasErrors {
		t.Fatal("Правила одного экземпляра повлияли на другой")
	}
}

func TestImportDiagnosticFilterKeepPreservesHistoryAndDeduplication(t *testing.T) {
	p := New(nil, parserInitialResult(t), nil, diagnostics.NewRegistry(), nil)
	p.version, p.processingID = "1.2", "current"
	old := lineDiagnostic(t, diagnostics.P012, 1, 1, 4)
	old.ID, old.Source, old.Message = "d1", "old", "История"
	p.result.Diagnostics = []model.Diagnostic{old}
	history := parserSnapshot(t, old)
	line := model.Line{Number: 1, Raw: "{x}", Elements: []model.Element{{Raw: "{x}", Start: 1, End: 4, ErrorIDs: []string{"d1"}}}}
	context := grammar.NewContext(line.Raw)
	first := lineDiagnostic(t, diagnostics.P012, 1, 1, 4)
	first.Message = "Первое обнаружение"
	context.AddDiagnostic(first)
	duplicate := first
	duplicate.Message = "Повтор"
	context.AddDiagnostic(duplicate)
	calls := 0
	p.diagnosticRules[diagnosticRuleKey{"1.2", diagnostics.P012}] = []diagnosticRule{func(diagnosticRuleInput) (diagnosticDecision, error) {
		calls++
		return diagnosticKeep, nil
	}}
	for range 2 {
		if err := p.importLineDiagnostics(context, &line); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 4 || len(p.result.Diagnostics) != 2 || parserSnapshot(t, p.result.Diagnostics[0]) != history ||
		p.result.Diagnostics[1].ID != "d2" || p.result.Diagnostics[1].Source != "current" || p.result.Diagnostics[1].Message != first.Message ||
		!slices.Equal(line.Elements[0].ErrorIDs, []string{"d1", "d2"}) || p.nextDiagnosticID != 3 {
		t.Fatalf("Нарушены история, дедупликация или ссылки: %+v; %+v", p.result.Diagnostics, line)
	}
	p.result.Lines = []model.Line{line}
	if err := checkElementLinks(p.result); err != nil {
		t.Fatal(err)
	}
}

func TestImportDiagnosticFilterErrorsStopBeforeRegistration(t *testing.T) {
	failure := errors.New("ошибка обработчика")
	for _, mode := range []string{"unknown code", "unknown code with rule", "handler error"} {
		t.Run(mode, func(t *testing.T) {
			p := New(nil, parserInitialResult(t), nil, diagnostics.NewRegistry(), nil)
			p.version, p.processingID = "1.2", "current"
			code := diagnostics.Code("UNKNOWN")
			if mode == "handler error" {
				code = diagnostics.P012
			}
			if mode != "unknown code" {
				p.diagnosticRules[diagnosticRuleKey{"1.2", code}] = []diagnosticRule{func(diagnosticRuleInput) (diagnosticDecision, error) {
					if mode == "handler error" {
						return diagnosticSuppress, failure
					}
					return diagnosticKeep, nil
				}}
			}
			line := model.Line{Number: 1, Raw: "x", Elements: []model.Element{{Raw: "x", Start: 1, End: 2}}}
			context := grammar.NewContext(line.Raw)
			context.AddDiagnostic(lineDiagnostic(t, code, 1, 1, 2))
			context.AddDiagnostic(lineDiagnostic(t, diagnostics.P003, 1, 1, 2))
			before := parserSnapshot(t, context.Diagnostics())
			err := p.importLineDiagnostics(context, &line)
			if err == nil || (mode == "handler error" && !errors.Is(err, failure)) || len(p.result.Diagnostics) != 0 ||
				p.nextDiagnosticID != 1 || len(line.Elements[0].ErrorIDs) != 0 || parserSnapshot(t, context.Diagnostics()) != before {
				t.Fatalf("Изменена обработка технической ошибки: %v", err)
			}
		})
	}
}

func TestLinkElementErrorRequiresExactRange(t *testing.T) {
	for _, tc := range []struct {
		name             string
		line, start, end int
		scope            diagnostics.Scope
		valid            bool
	}{
		{"unicode", 2, 1, 2, diagnostics.ScopeElement, true}, {"insertion", 2, 2, 2, diagnostics.ScopeElement, true},
		{"wrong line", 1, 1, 2, diagnostics.ScopeElement, false}, {"wrong end", 2, 1, 3, diagnostics.ScopeElement, false},
		{"line scope", 2, 1, 2, diagnostics.ScopeLine, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := model.Line{Number: 2, Raw: "я", Elements: []model.Element{{Raw: "я", Start: 1, End: 2}, {Start: 2, End: 2}}}
			diagnostic := lineDiagnostic(t, diagnostics.P015, tc.line, tc.start, tc.end)
			diagnostic.ID, diagnostic.DiagnosticScope = "id", tc.scope
			for range 2 {
				if err := linkElementError(&line, diagnostic); (err == nil) != tc.valid {
					t.Fatalf("linkElementError = %v", err)
				}
			}
			count := len(line.Elements[0].ErrorIDs) + len(line.Elements[1].ErrorIDs)
			if (tc.valid && count != 1) || (!tc.valid && count != 0) {
				t.Fatal("Неверная или повторная ссылка")
			}
		})
	}
	diagnostic := lineDiagnostic(t, diagnostics.P015, 1, 1, 1)
	diagnostic.ID, diagnostic.Location = "id", nil
	if err := linkElementError(&model.Line{Number: 1}, diagnostic); err == nil {
		t.Fatal("Принята отсутствующая Location")
	}
}

func TestOrderDiagnosticsPreservesIDsAndStablePositionOrder(t *testing.T) {
	result := &model.Result{Processing: []model.Processing{{ID: "z"}, {ID: "a"}}}
	for _, tc := range []struct {
		id, source        string
		line, column, end int
	}{
		{"later-line", "a", 2, 1, 2}, {"later-column", "a", 1, 3, 4}, {"first-tie", "a", 1, 1, 3},
		{"old", "z", 9, 1, 2}, {"second-tie", "a", 1, 1, 3}, {"shorter", "a", 1, 1, 2}, {"unlocated", "a", 0, 0, 0},
	} {
		diagnostic := lineDiagnostic(t, diagnostics.P015, tc.line, tc.column, tc.end)
		diagnostic.ID, diagnostic.Source = tc.id, tc.source
		if tc.line == 0 {
			diagnostic.Location = nil
		}
		result.Diagnostics = append(result.Diagnostics, diagnostic)
	}
	before := make(map[string]string)
	for _, diagnostic := range result.Diagnostics {
		before[diagnostic.ID] = parserSnapshot(t, diagnostic)
	}
	result.Lines = []model.Line{{Number: 1, Elements: []model.Element{{Start: 1, End: 3, ErrorIDs: []string{"second-tie", "first-tie"}}}}}
	orderDiagnostics(result)
	want := []string{"old", "unlocated", "first-tie", "second-tie", "shorter", "later-column", "later-line"}
	for i, diagnostic := range result.Diagnostics {
		if diagnostic.ID != want[i] || parserSnapshot(t, diagnostic) != before[diagnostic.ID] {
			t.Fatalf("Неверная сортировка: %+v", result.Diagnostics)
		}
	}
	if !slices.Equal(result.Lines[0].Elements[0].ErrorIDs, []string{"first-tie", "second-tie"}) {
		t.Fatal("Порядок ErrorIDs не согласован")
	}
	ordered := parserSnapshot(t, result)
	orderDiagnostics(result)
	if parserSnapshot(t, result) != ordered {
		t.Fatal("Сортировка нестабильна")
	}
}

func TestSetErrorFlagsDoesNotPropagateAcrossRelationships(t *testing.T) {
	parent := 1
	for _, tc := range []struct {
		name                 string
		severity             diagnostics.Severity
		line                 int
		linked, wantDocument bool
		wantLines            []bool
	}{
		{"parent", diagnostics.SeverityError, 1, false, true, []bool{true, false, false}},
		{"child", diagnostics.SeverityError, 2, false, true, []bool{false, true, false}},
		{"document", diagnostics.SeverityError, 0, false, true, []bool{false, false, false}},
		{"element link", diagnostics.SeverityError, 0, true, true, []bool{false, true, false}},
		{"warning", diagnostics.SeverityWarning, 2, true, false, []bool{false, false, false}},
		{"recommendation", diagnostics.SeverityRecommendation, 1, false, false, []bool{false, false, false}},
		{"empty", "", 0, false, false, []bool{false, false, false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := &model.Result{Lines: []model.Line{{Number: 1, HasErrors: true}, {Number: 2, ParentLine: &parent, HasErrors: true}, {Number: 3, HasErrors: true}}}
			result.Document.HasErrors = true
			diagnostic := lineDiagnostic(t, diagnostics.P011, tc.line, 1, 2)
			diagnostic.ID, diagnostic.SeverityLevel = "error", tc.severity
			diagnostic.Location.End.Line = 3
			diagnostic.RelatedLocations = []model.Location{*lineDiagnostic(t, diagnostics.P011, 3, 1, 2).Location}
			if tc.line == 0 {
				diagnostic.Location = nil
			}
			if tc.severity != "" {
				result.Diagnostics = []model.Diagnostic{diagnostic}
			}
			if tc.linked {
				result.Lines[1].Elements = []model.Element{{ErrorIDs: []string{"error"}}}
			}
			setErrorFlags(result)
			if result.Document.HasErrors != tc.wantDocument {
				t.Fatal("Неверный признак документа")
			}
			for i, line := range result.Lines {
				if line.HasErrors != tc.wantLines[i] {
					t.Fatalf("Неверный признак строки %d", line.Number)
				}
			}
		})
	}
}

func lineDiagnostic(t *testing.T, code diagnostics.Code, line, start, end int) model.Diagnostic {
	t.Helper()
	return model.Diagnostic{DiagnosticCode: code, DiagnosticScope: diagnostics.ScopeElement, SeverityLevel: diagnostics.SeverityError,
		Location: &model.Location{Start: model.Position{Line: line, Column: start}, End: model.Position{Line: line, Column: end}}}
}
