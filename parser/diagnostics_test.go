package parser

import (
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
		{diagnostics.P005, diagnostics.ScopeLine, false}, {diagnostics.P006, diagnostics.ScopeLine, false},
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

func TestImportP008ElementExceptionOnlyForStandaloneBrace(t *testing.T) {
	for _, raw := range []string{"{", " \t{ \t", "@editor {", "{{"} {
		t.Run(raw, func(t *testing.T) {
			result := parserInitialResult(t)
			p := New(nil, result, nil, diagnostics.NewRegistry(), nil)
			p.processingID = "current"
			start := 1
			if raw == " \t{ \t" {
				start = 3
			}
			line := model.Line{Number: 1, Raw: raw, Elements: []model.Element{{Raw: "{", Start: start, End: start + 1}}}
			context := grammar.NewContext(raw)
			context.AddDiagnostic(lineDiagnostic(t, diagnostics.P008, 1, start, start+1))
			if err := p.importLineDiagnostics(context, &line); err != nil {
				t.Fatal(err)
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
