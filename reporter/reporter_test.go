package reporter_test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
	"github.com/verdoga/dsl-parser/reporter"
	"github.com/verdoga/dsl-parser/storage"
)

func TestReporterRecordsOutcomesAndFinishesRun(t *testing.T) {
	count := 3
	result := model.Result{Document: model.Document{LineCount: &count}}
	inputs := []struct {
		report storage.Report
		want   string
	}{
		{
			report: storage.Report{SourcePath: "/docs/created.txt", Action: storage.ActionCreated},
			want:   `УСПЕХ: СОЗДАН | путь: "/docs/created.txt" | строк: 3 | ошибок: 0`,
		},
		{
			report: storage.Report{SourcePath: "/docs/replaced.txt", Action: storage.ActionReplaced},
			want:   `УСПЕХ: ЗАМЕНЁН | путь: "/docs/replaced.txt" | строк: 3 | ошибок: 0`,
		},
		{
			report: storage.Report{SourcePath: "/docs/diagnostics.txt", Action: storage.ActionCreated, ErrorCount: 7},
			want:   `УСПЕХ: СОЗДАН | путь: "/docs/diagnostics.txt" | строк: 3 | ошибок: 7`,
		},
		{
			report: storage.Report{SourcePath: "/docs/denied.txt", Action: storage.ActionNotWritten, Err: errors.New("замена запрещена")},
			want:   `ОШИБКА: НЕ СОЗДАН | путь: "/docs/denied.txt" | строк: 3 | ошибок: 0 | причина: замена запрещена`,
		},
		{
			report: storage.Report{SourcePath: "/docs/cleanup-created.txt", Action: storage.ActionCreated, Err: errors.New("ошибка очистки")},
			want:   `ОШИБКА: СОЗДАН | путь: "/docs/cleanup-created.txt" | строк: 3 | ошибок: 0 | причина: ошибка очистки`,
		},
		{
			report: storage.Report{SourcePath: "/docs/cleanup-replaced.txt", Action: storage.ActionReplaced, Err: errors.New("ошибка очистки")},
			want:   `ОШИБКА: ЗАМЕНЁН | путь: "/docs/cleanup-replaced.txt" | строк: 3 | ошибок: 0 | причина: ошибка очистки`,
		},
	}
	tests := []struct {
		name      string
		inputs    []int
		wantTotal string
		wantCode  int
	}{
		{name: "empty run", wantTotal: "ИТОГО | всего: 0 | успех: 0 | ошибка: 0"},
		{name: "successful writes", inputs: []int{0, 1}, wantTotal: "ИТОГО | всего: 2 | успех: 2 | ошибка: 0"},
		{name: "diagnostics do not fail successful write", inputs: []int{2}, wantTotal: "ИТОГО | всего: 1 | успех: 1 | ошибка: 0"},
		{name: "refused write", inputs: []int{3}, wantTotal: "ИТОГО | всего: 1 | успех: 0 | ошибка: 1", wantCode: 1},
		{name: "created with cleanup failure", inputs: []int{4}, wantTotal: "ИТОГО | всего: 1 | успех: 0 | ошибка: 1", wantCode: 1},
		{name: "replaced with cleanup failure", inputs: []int{5}, wantTotal: "ИТОГО | всего: 1 | успех: 0 | ошибка: 1", wantCode: 1},
		{name: "mixed sequence", inputs: []int{0, 3, 1, 4, 2, 5}, wantTotal: "ИТОГО | всего: 6 | успех: 3 | ошибка: 3", wantCode: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var r *reporter.Reporter
			if output := captureReporterOutput(t, func() { r = reporter.New() }); output != "" {
				t.Fatalf("New вывел %q", output)
			}
			for position, index := range test.inputs {
				input := inputs[index]
				output := captureReporterOutput(t, func() { r.Record(result, input.report) })
				if output != input.want+"\n" || strings.Count(output, "\n") != 1 {
					t.Fatalf("Record %d вывел %q, требуется %q", position+1, output, input.want+"\n")
				}
			}
			var code int
			output := captureReporterOutput(t, func() { code = r.Finish() })
			if output != test.wantTotal+"\n" || strings.Count(output, "\n") != 1 {
				t.Fatalf("Finish вывел %q, требуется %q", output, test.wantTotal+"\n")
			}
			if code != test.wantCode {
				t.Errorf("код возврата = %d, требуется %d", code, test.wantCode)
			}
			var total, success, failed int
			n, err := fmt.Sscanf(output, "ИТОГО | всего: %d | успех: %d | ошибка: %d", &total, &success, &failed)
			if err != nil || n != 3 || total != len(test.inputs) || total != success+failed {
				t.Errorf("несогласованные счётчики: %q; ошибка разбора: %v", output, err)
			}
		})
	}
}

func TestReporterPrintsCurrentFatalAndEscapedReasonOnOneLine(t *testing.T) {
	count := 10
	result := model.Result{
		Document:   model.Document{LineCount: &count},
		Processing: []model.Processing{{ID: "old"}, {ID: "current"}},
		Diagnostics: []model.Diagnostic{
			{ID: "historical", Source: "old", DiagnosticCode: diagnostics.IO001, Fatal: true},
			{ID: "current-fatal", Source: "current", DiagnosticCode: diagnostics.P014, Fatal: true},
		},
	}
	report := storage.Report{Action: storage.ActionNotWritten, ErrorCount: 2, Err: errors.New("первая\r\nвторая")}
	var code int
	output := captureReporterOutput(t, func() {
		r := reporter.New()
		r.Record(result, report)
		code = r.Finish()
	})
	want := `ОШИБКА: НЕ СОЗДАН | путь: неизвестен | строк: нет данных | ошибок: 2 | причина: первая\r\nвторая | фатальная ошибка: P014` + "\n" +
		"ИТОГО | всего: 1 | успех: 0 | ошибка: 1\n"
	if output != want || code != 1 {
		t.Fatalf("вывод = %q, код = %d; требуется %q, код 1", output, code, want)
	}
}

func TestReporterInstancesKeepIndependentCounters(t *testing.T) {
	var firstCode, secondCode int
	output := captureReporterOutput(t, func() {
		first := reporter.New()
		first.Record(model.Result{}, storage.Report{Action: storage.ActionNotWritten, Err: errors.New("отказ")})
		second := reporter.New()
		first.Record(model.Result{}, storage.Report{SourcePath: "/docs/first.txt", Action: storage.ActionCreated})
		second.Record(model.Result{}, storage.Report{SourcePath: "/docs/second.txt", Action: storage.ActionReplaced})
		firstCode = first.Finish()
		secondCode = second.Finish()
	})
	want := "ОШИБКА: НЕ СОЗДАН | путь: неизвестен | строк: нет данных | ошибок: 0 | причина: отказ\n" +
		`УСПЕХ: СОЗДАН | путь: "/docs/first.txt" | строк: нет данных | ошибок: 0` + "\n" +
		`УСПЕХ: ЗАМЕНЁН | путь: "/docs/second.txt" | строк: нет данных | ошибок: 0` + "\n" +
		"ИТОГО | всего: 2 | успех: 1 | ошибка: 1\n" +
		"ИТОГО | всего: 1 | успех: 1 | ошибка: 0\n"
	if output != want {
		t.Errorf("вывод = %q, требуется %q", output, want)
	}
	if firstCode != 1 || secondCode != 0 {
		t.Errorf("коды возврата = (%d, %d), требуется (1, 0)", firstCode, secondCode)
	}
}

func captureReporterOutput(t *testing.T, run func()) string {
	t.Helper()
	capture, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = capture
	defer func() {
		os.Stdout = stdout
		if err := capture.Close(); err != nil {
			t.Errorf("закрытие перехвата stdout: %v", err)
		}
	}()
	run()
	payload, err := os.ReadFile(capture.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
