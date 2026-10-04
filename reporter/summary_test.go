package reporter

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
	"github.com/verdoga/dsl-parser/storage"
)

func TestCurrentDiagnosticsSelectsLastProcessingInModelOrder(t *testing.T) {
	later, earlier := "2026-10-04T12:00:00Z", "2026-10-03T12:00:00Z"
	entries := []model.Diagnostic{
		{ID: "old-first", Source: "old", DiagnosticCode: diagnostics.P001, Fatal: true},
		{ID: "current-first", Source: "current", DiagnosticCode: diagnostics.P002, Fatal: true},
		{ID: "old-last", Source: "old"},
		{ID: "current-last", Source: "current", DiagnosticCode: diagnostics.P014, Fatal: true},
	}
	tests := []struct {
		name       string
		processing []model.Processing
		want       []int
	}{
		{name: "nil history"},
		{name: "empty history", processing: []model.Processing{}},
		{name: "single run", processing: []model.Processing{{ID: "old"}}, want: []int{0, 2}},
		{name: "last position overrides timestamp", processing: []model.Processing{
			{ID: "old", StartedAt: &later}, {ID: "current", StartedAt: &earlier},
		}, want: []int{1, 3}},
		{name: "last run without diagnostics", processing: []model.Processing{{ID: "old"}, {ID: "empty"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := model.Result{Processing: test.processing, Diagnostics: entries}
			before := summaryModelSnapshot(t, result)
			got := currentDiagnostics(result)
			if len(got) != len(test.want) {
				t.Fatalf("число диагностик = %d, требуется %d", len(got), len(test.want))
			}
			for i, index := range test.want {
				wantJSON, err := json.Marshal(entries[index])
				if err != nil {
					t.Fatal(err)
				}
				gotJSON, err := json.Marshal(got[i])
				if err != nil {
					t.Fatal(err)
				}
				if string(gotJSON) != string(wantJSON) {
					t.Errorf("диагностика %d = %s, требуется %s", i, gotJSON, wantJSON)
				}
			}
			if summaryModelSnapshot(t, result) != before {
				t.Fatal("модель изменена")
			}
		})
	}
}

func TestEarlyFatalRequiresSupportedCodeAndFatalFlag(t *testing.T) {
	tests := []struct {
		code diagnostics.Code
		want bool
	}{
		{diagnostics.IO001, true}, {diagnostics.P001, true}, {diagnostics.P002, true},
		{diagnostics.P013, true}, {diagnostics.P014, true},
		{diagnostics.P003, false}, {diagnostics.Code("CUSTOM001"), false}, {diagnostics.Code(""), false},
	}
	for _, test := range tests {
		for _, fatal := range []bool{false, true} {
			name := string(test.code) + "/nonfatal"
			if fatal {
				name = string(test.code) + "/fatal"
			}
			t.Run(name, func(t *testing.T) {
				result := model.Result{Diagnostics: []model.Diagnostic{
					{ID: "candidate", DiagnosticCode: test.code, Fatal: fatal},
				}}
				before := summaryModelSnapshot(t, result)
				got := earlyFatal(result.Diagnostics)
				if test.want && fatal {
					if got != &result.Diagnostics[0] {
						t.Fatal("требуется указатель на исходную диагностику")
					}
				} else if got != nil {
					t.Fatalf("неподходящая диагностика выбрана: %+v", got)
				}
				if summaryModelSnapshot(t, result) != before {
					t.Fatal("модель изменена")
				}
			})
		}
	}
}

func TestEarlyFatalReturnsFirstMatchingElement(t *testing.T) {
	entries := []model.Diagnostic{
		{ID: "nonfatal", DiagnosticCode: diagnostics.P001},
		{ID: "other", DiagnosticCode: diagnostics.P003, Fatal: true},
		{ID: "first", DiagnosticCode: diagnostics.P014, Fatal: true},
		{ID: "second", DiagnosticCode: diagnostics.IO001, Fatal: true},
	}
	if got := earlyFatal(entries); got != &entries[2] {
		t.Fatalf("выбрана не первая подходящая запись: %+v", got)
	}
	for _, empty := range [][]model.Diagnostic{nil, {}} {
		if got := earlyFatal(empty); got != nil {
			t.Fatalf("пустая выборка вернула %+v", got)
		}
	}
}

func TestSummarizeFilePreservesReportAndModelOwnership(t *testing.T) {
	zero, count := 0, 42
	modelPath := "/model/ignored.txt"
	failure := errors.New("отказ\r\nсохранения")
	tests := []struct {
		name        string
		lineCount   *int
		processing  []model.Processing
		entries     []model.Diagnostic
		fatalIndex  int
		unknownPath bool
	}{
		{name: "unknown count", fatalIndex: -1},
		{name: "zero count", lineCount: &zero, fatalIndex: -1},
		{name: "known count", lineCount: &count, fatalIndex: -1},
		{name: "unknown report path", lineCount: &count, fatalIndex: -1, unknownPath: true},
		{name: "historical fatal", lineCount: &count, fatalIndex: -1,
			processing: []model.Processing{{ID: "old"}, {ID: "current"}},
			entries:    []model.Diagnostic{{ID: "old-fatal", Source: "old", DiagnosticCode: diagnostics.P001, Fatal: true}}},
		{name: "no processing", lineCount: &count, fatalIndex: -1,
			entries: []model.Diagnostic{{ID: "orphan", DiagnosticCode: diagnostics.IO001, Fatal: true}}},
		{name: "other fatal code", lineCount: &count, fatalIndex: -1,
			processing: []model.Processing{{ID: "current"}},
			entries:    []model.Diagnostic{{ID: "other", Source: "current", DiagnosticCode: diagnostics.P003, Fatal: true}}},
		{name: "first current fatal belongs to model", lineCount: &count, fatalIndex: 2,
			processing: []model.Processing{{ID: "old"}, {ID: "current"}},
			entries: []model.Diagnostic{
				{ID: "old-fatal", Source: "old", DiagnosticCode: diagnostics.IO001, Fatal: true},
				{ID: "nonfatal", Source: "current", DiagnosticCode: diagnostics.P001},
				{ID: "first", Source: "current", DiagnosticCode: diagnostics.P013, Fatal: true},
				{ID: "old-last", Source: "old"},
				{ID: "second", Source: "current", DiagnosticCode: diagnostics.P014, Fatal: true},
			}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := model.Result{
				Document:   model.Document{FilePath: &modelPath, LineCount: test.lineCount},
				Processing: test.processing, Diagnostics: test.entries,
				Lines: []model.Line{{Number: 1}},
			}
			before := summaryModelSnapshot(t, result)
			report := storage.Report{
				SourcePath: "/storage/source.txt", TargetPath: "/storage/source.json",
				Action: storage.ActionReplaced, ErrorCount: 17, Err: failure,
			}
			if test.unknownPath {
				report.SourcePath = ""
			}
			got := summarizeFile(result, report)
			if got.path != report.SourcePath || got.action != report.Action ||
				got.errors != report.ErrorCount || got.failure != failure {
				t.Fatalf("сведения отчёта не сохранены: %+v", got)
			}
			if test.fatalIndex >= 0 {
				if got.fatal != &result.Diagnostics[test.fatalIndex] || got.lineCount != nil {
					t.Fatalf("неверная фатальная запись или число строк: %+v", got)
				}
			} else if got.fatal != nil || got.lineCount != test.lineCount {
				t.Fatalf("изменено число строк или добавлена фатальная пометка: %+v", got)
			}
			if summaryModelSnapshot(t, result) != before {
				t.Fatal("модель изменена")
			}
		})
	}
}

func TestSummaryFailureDependsOnlyOnStorageOutcome(t *testing.T) {
	failure := errors.New("ошибка записи или очистки")
	tests := []struct {
		name   string
		action storage.Action
		err    error
		want   bool
	}{
		{"not written", storage.ActionNotWritten, failure, true},
		{"created", storage.ActionCreated, nil, false},
		{"replaced", storage.ActionReplaced, nil, false},
		{"created with cleanup failure", storage.ActionCreated, failure, true},
		{"replaced with cleanup failure", storage.ActionReplaced, failure, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := model.Result{
				Processing:  []model.Processing{{ID: "current"}},
				Diagnostics: []model.Diagnostic{{ID: "fatal", Source: "current", DiagnosticCode: diagnostics.P002, Fatal: true}},
			}
			report := storage.Report{Action: test.action, Err: test.err, ErrorCount: 5}
			summary := summarizeFile(result, report)
			if summary.action != test.action || summary.failure != test.err || summary.errors != 5 {
				t.Fatalf("фактический итог изменён: %+v", summary)
			}
			if summary.fatal != &result.Diagnostics[0] || summary.lineCount != nil {
				t.Fatalf("ранняя фатальная диагностика потеряна: %+v", summary)
			}
			if got := isFailure(summary); got != test.want {
				t.Errorf("isFailure = %t, требуется %t", got, test.want)
			}
		})
	}
}

func summaryModelSnapshot(t *testing.T, result model.Result) string {
	t.Helper()
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
