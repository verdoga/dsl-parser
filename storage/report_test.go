package storage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestActionValues(t *testing.T) {
	tests := []struct {
		action Action
		want   string
	}{
		{ActionNotWritten, "not-written"},
		{ActionCreated, "created"},
		{ActionReplaced, "replaced"},
	}
	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			if string(test.action) != test.want {
				t.Errorf("Action = %q, требуется %q", test.action, test.want)
			}
		})
	}
}

func TestNewReportPreservesAttemptResult(t *testing.T) {
	writeErr := errors.New("запись запрещена")
	cleanupErr := errors.New("не удалось удалить временный файл")
	tests := []struct {
		name string
		want Report
	}{
		{
			name: "not written",
			want: Report{SourcePath: "/docs/lesson.txt", TargetPath: "/docs/lesson.json", Action: ActionNotWritten, ErrorCount: 2, Err: writeErr},
		},
		{
			name: "created",
			want: Report{SourcePath: "/docs/lesson.txt", TargetPath: "/docs/lesson.json", Action: ActionCreated},
		},
		{
			name: "replaced with nonfatal errors",
			want: Report{SourcePath: "/docs/lesson.txt", TargetPath: "/docs/lesson.json", Action: ActionReplaced, ErrorCount: 3},
		},
		{
			name: "created with cleanup error",
			want: Report{SourcePath: "/docs/lesson.txt", TargetPath: "/docs/lesson.json", Action: ActionCreated, ErrorCount: 1, Err: cleanupErr},
		},
		{
			name: "replaced with cleanup error",
			want: Report{SourcePath: "/docs/lesson.txt", TargetPath: "/docs/lesson.json", Action: ActionReplaced, ErrorCount: 4, Err: cleanupErr},
		},
		{
			name: "unknown paths",
			want: Report{Action: ActionNotWritten, Err: writeErr},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := test.want
			got := NewReport(want.SourcePath, want.TargetPath, want.Action, want.ErrorCount, want.Err)
			if got.SourcePath != want.SourcePath || got.TargetPath != want.TargetPath {
				t.Errorf("пути = (%q, %q), требуется (%q, %q)", got.SourcePath, got.TargetPath, want.SourcePath, want.TargetPath)
			}
			if got.Action != want.Action || got.ErrorCount != want.ErrorCount {
				t.Errorf("действие и число ошибок = (%q, %d), требуется (%q, %d)", got.Action, got.ErrorCount, want.Action, want.ErrorCount)
			}
			if got.Err != want.Err {
				t.Errorf("Err = %v, требуется исходная ошибка %v", got.Err, want.Err)
			}
		})
	}
}

func TestNewReportPreservesPathsWithoutFilesystemValidation(t *testing.T) {
	directory := t.TempDir()
	separator := string(filepath.Separator)
	sourcePath := directory + separator + "missing" + separator + ".." + separator + "lesson.txt"
	targetPath := filepath.Join(directory, "missing", "lesson.json")

	got := NewReport(sourcePath, targetPath, ActionCreated, 0, nil)
	if got.SourcePath != sourcePath || got.TargetPath != targetPath {
		t.Fatalf("пути изменены: (%q, %q), требуется (%q, %q)", got.SourcePath, got.TargetPath, sourcePath, targetPath)
	}
	if got.Action != ActionCreated || got.Err != nil {
		t.Errorf("итог изменён из-за несуществующих путей: %+v", got)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("конструктор создал объекты в каталоге: %v", entries)
	}
}

func TestReportJSONOmitsErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "nil error"},
		{name: "filesystem error", err: &os.PathError{Op: "remove", Path: "backup", Err: os.ErrPermission}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := NewReport("/docs/lesson.txt", "/docs/lesson.json", ActionReplaced, 2, test.err)
			payload, err := json.Marshal(report)
			if err != nil {
				t.Fatalf("сериализация отчёта: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(payload, &fields); err != nil {
				t.Fatalf("чтение JSON отчёта: %v", err)
			}
			if _, exists := fields["Err"]; exists {
				t.Errorf("Err попал в JSON: %s", payload)
			}
			if len(fields) != 4 {
				t.Errorf("в JSON требуется четыре поля отчёта без ошибки: %s", payload)
			}
			var decoded Report
			if err := json.Unmarshal(payload, &decoded); err != nil {
				t.Fatalf("чтение отчёта: %v", err)
			}
			if decoded.SourcePath != report.SourcePath || decoded.TargetPath != report.TargetPath ||
				decoded.Action != report.Action || decoded.ErrorCount != report.ErrorCount || decoded.Err != nil {
				t.Errorf("JSON не сохранил поля отчёта без ошибки: %+v", decoded)
			}
		})
	}
}
