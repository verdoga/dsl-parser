package reporter

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
	"github.com/verdoga/dsl-parser/storage"
)

func TestActionTextTranslatesStorageActions(t *testing.T) {
	tests := []struct {
		action storage.Action
		want   string
	}{
		{storage.ActionCreated, "СОЗДАН"},
		{storage.ActionReplaced, "ЗАМЕНЁН"},
		{storage.ActionNotWritten, "НЕ СОЗДАН"},
	}
	for _, test := range tests {
		t.Run(string(test.action), func(t *testing.T) {
			if got := actionText(test.action); got != test.want {
				t.Errorf("actionText = %q, требуется %q", got, test.want)
			}
		})
	}
}

func TestFormatFileLineMatchesContract(t *testing.T) {
	zero, count := 0, 12
	failure := errors.New("запись запрещена")
	cleanup := errors.New("не удалось удалить временный файл")
	fatal := model.Diagnostic{ID: "fatal", DiagnosticCode: diagnostics.P013, Fatal: true}
	tests := []struct {
		name    string
		summary fileSummary
		want    string
	}{
		{
			name:    "created empty file",
			summary: fileSummary{path: "/уроки/пустой.txt", lineCount: &zero, action: storage.ActionCreated},
			want:    `УСПЕХ: СОЗДАН | путь: "/уроки/пустой.txt" | строк: 0 | ошибок: 0`,
		},
		{
			name:    "replaced with diagnostics",
			summary: fileSummary{path: "/уроки/текст.txt", lineCount: &count, errors: 3, action: storage.ActionReplaced},
			want:    `УСПЕХ: ЗАМЕНЁН | путь: "/уроки/текст.txt" | строк: 12 | ошибок: 3`,
		},
		{
			name:    "not written",
			summary: fileSummary{path: "/уроки/текст.txt", lineCount: &count, errors: 2, action: storage.ActionNotWritten, failure: failure},
			want:    `ОШИБКА: НЕ СОЗДАН | путь: "/уроки/текст.txt" | строк: 12 | ошибок: 2 | причина: запись запрещена`,
		},
		{
			name:    "created with cleanup failure",
			summary: fileSummary{path: "/уроки/текст.txt", lineCount: &zero, action: storage.ActionCreated, failure: cleanup},
			want:    `ОШИБКА: СОЗДАН | путь: "/уроки/текст.txt" | строк: 0 | ошибок: 0 | причина: не удалось удалить временный файл`,
		},
		{
			name:    "replaced with cleanup failure",
			summary: fileSummary{path: "/уроки/текст.txt", lineCount: &count, errors: 1, action: storage.ActionReplaced, failure: cleanup},
			want:    `ОШИБКА: ЗАМЕНЁН | путь: "/уроки/текст.txt" | строк: 12 | ошибок: 1 | причина: не удалось удалить временный файл`,
		},
		{
			name:    "unknown path and count on failure",
			summary: fileSummary{errors: 1, action: storage.ActionNotWritten, failure: failure},
			want:    `ОШИБКА: НЕ СОЗДАН | путь: неизвестен | строк: нет данных | ошибок: 1 | причина: запись запрещена`,
		},
		{
			name:    "unknown path and count on success",
			summary: fileSummary{action: storage.ActionCreated},
			want:    `УСПЕХ: СОЗДАН | путь: неизвестен | строк: нет данных | ошибок: 0`,
		},
		{
			name:    "quoted path",
			summary: fileSummary{path: "/уроки/\"цитата\"\\путь\r\n\t.txt", lineCount: &count, action: storage.ActionCreated},
			want:    `УСПЕХ: СОЗДАН | путь: "/уроки/\"цитата\"\\путь\r\n\t.txt" | строк: 12 | ошибок: 0`,
		},
		{
			name:    "fatal suffix after failure reason",
			summary: fileSummary{path: "/уроки/текст.txt", errors: 1, action: storage.ActionNotWritten, failure: failure, fatal: &fatal},
			want:    `ОШИБКА: НЕ СОЗДАН | путь: "/уроки/текст.txt" | строк: нет данных | ошибок: 1 | причина: запись запрещена | фатальная ошибка: P013`,
		},
		{
			name:    "fatal suffix on success",
			summary: fileSummary{path: "/уроки/текст.txt", errors: 1, action: storage.ActionReplaced, fatal: &fatal},
			want:    `УСПЕХ: ЗАМЕНЁН | путь: "/уроки/текст.txt" | строк: нет данных | ошибок: 1 | фатальная ошибка: P013`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := test.summary
			beforeModel := summaryModelSnapshot(t, model.Result{
				Document:    model.Document{LineCount: test.summary.lineCount},
				Diagnostics: []model.Diagnostic{fatal},
			})
			got := formatFileLine(test.summary)
			if got != test.want {
				t.Errorf("строка = %q, требуется %q", got, test.want)
			}
			if strings.ContainsAny(got, "\r\n") {
				t.Errorf("результат содержит физический перевод строки: %q", got)
			}
			if test.summary != before || summaryModelSnapshot(t, model.Result{
				Document:    model.Document{LineCount: test.summary.lineCount},
				Diagnostics: []model.Diagnostic{fatal},
			}) != beforeModel {
				t.Fatal("форматирование изменило входные данные")
			}
		})
	}
}

func TestFormatFileLineEscapesReasonLineBreaks(t *testing.T) {
	tests := []struct {
		name, reason, want string
	}{
		{"CR", "первая\rвторая", `первая\rвторая`},
		{"LF", "первая\nвторая", `первая\nвторая`},
		{"CRLF", "первая\r\nвторая", `первая\r\nвторая`},
		{"joined errors", errors.Join(errors.New("первая"), errors.New("вторая")).Error(), `первая\nвторая`},
		{"other text preserved", "\"текст\"\\путь\tконец", "\"текст\"\\путь\tконец"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			failure := errors.New(test.reason)
			got := formatFileLine(fileSummary{action: storage.ActionNotWritten, failure: failure})
			want := "ОШИБКА: НЕ СОЗДАН | путь: неизвестен | строк: нет данных | ошибок: 0 | причина: " + test.want
			if got != want || strings.ContainsAny(got, "\r\n") {
				t.Errorf("строка = %q, требуется %q", got, want)
			}
			if failure.Error() != test.reason {
				t.Fatal("исходная ошибка изменена")
			}
		})
	}
}

func TestFormatTotalLineUsesProvidedCounters(t *testing.T) {
	tests := []struct {
		name                   string
		total, success, failed int
		want                   string
	}{
		{"empty", 0, 0, 0, "ИТОГО | всего: 0 | успех: 0 | ошибка: 0"},
		{"successes", 3, 3, 0, "ИТОГО | всего: 3 | успех: 3 | ошибка: 0"},
		{"failures", 2, 0, 2, "ИТОГО | всего: 2 | успех: 0 | ошибка: 2"},
		{"mixed", 12, 7, 5, "ИТОГО | всего: 12 | успех: 7 | ошибка: 5"},
		{"no recalculation", 9, 2, 3, "ИТОГО | всего: 9 | успех: 2 | ошибка: 3"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := formatTotalLine(test.total, test.success, test.failed); got != test.want {
				t.Errorf("итог = %q, требуется %q", got, test.want)
			}
		})
	}
}

func TestFormattingDoesNotPrint(t *testing.T) {
	capture, err := os.CreateTemp(t.TempDir(), "console")
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = capture, capture
	defer func() {
		os.Stdout, os.Stderr = stdout, stderr
		capture.Close()
	}()
	actionText(storage.ActionCreated)
	formatFileLine(fileSummary{action: storage.ActionCreated})
	formatFileLine(fileSummary{action: storage.ActionNotWritten, failure: errors.New("отказ")})
	formatTotalLine(2, 1, 1)
	info, err := capture.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("форматирование вывело %d байт в консоль", info.Size())
	}
}
