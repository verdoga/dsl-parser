package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/verdoga/dsl-parser/console"
	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
	"github.com/verdoga/dsl-parser/reporter"
)

func TestNewResultInitializesIndependentModels(t *testing.T) {
	r := &runner{version: "test-version"}
	path := filepath.Join(t.TempDir(), "source.TXT")
	started := time.Date(2026, 10, 4, 15, 20, 30, 0, time.FixedZone("UTC+3", 3*60*60))
	first, second := r.newResult(path, started), r.newResult(path, started)
	doc := first.Document
	if first.Format != model.FormatVersion1 || doc.FilePath == nil || *doc.FilePath != path || doc.FileName == nil || *doc.FileName != "source.TXT" || doc.HasErrors {
		t.Fatalf("Неверная начальная модель: %+v", first)
	}
	if doc.DSLVersion != nil || doc.LineCount != nil || doc.Encoding != nil || doc.HasBOM != nil || doc.ByteLength != nil || doc.SHA256 != nil {
		t.Fatal("Неизвестные сведения документа должны оставаться nil")
	}
	metadata := doc.Metadata
	if metadata.DocumentID != nil || metadata.Title != nil || metadata.Subtitle != nil || metadata.Section != nil || metadata.Order != nil {
		t.Fatal("Начальная модель содержит неизвестные метаданные")
	}
	if metadata.ResourceDirs == nil || len(metadata.ResourceDirs) != 0 || first.Lines == nil || len(first.Lines) != 0 || first.Diagnostics == nil || len(first.Diagnostics) != 0 || len(first.Processing) != 1 {
		t.Fatal("Неверные начальные срезы модели")
	}
	processing := first.Processing[0]
	if processing.ID != "p1" || processing.Tool != "dsl-parser" || processing.Version == nil || *processing.Version != r.version || processing.StartedAt == nil || *processing.StartedAt != "2026-10-04T12:20:30Z" || processing.DurationMs != nil {
		t.Fatalf("Неверная запись обработки: %+v", processing)
	}
	*first.Document.FilePath, *first.Document.FileName = "changed", "changed"
	*first.Processing[0].Version, *first.Processing[0].StartedAt = "changed", "changed"
	first.Processing[0].ID = "changed"
	first.Document.Metadata.ResourceDirs = append(first.Document.Metadata.ResourceDirs, "changed")
	first.Lines = append(first.Lines, model.Line{Number: 1})
	first.Diagnostics = append(first.Diagnostics, model.Diagnostic{ID: "changed"})
	if *second.Document.FilePath != path || *second.Document.FileName != "source.TXT" || *second.Processing[0].Version != "test-version" || *second.Processing[0].StartedAt != "2026-10-04T12:20:30Z" || second.Processing[0].ID != "p1" || r.version != "test-version" {
		t.Fatal("Изменение результата затронуло другой результат или runner")
	}
	if len(second.Document.Metadata.ResourceDirs) != 0 || len(second.Lines) != 0 || len(second.Diagnostics) != 0 {
		t.Fatal("Модели разделяют накопленные записи")
	}
}

func TestFinishProcessingMeasuresNonnegativeMilliseconds(t *testing.T) {
	for _, offset := range []time.Duration{-1500 * time.Millisecond, 0, time.Hour} {
		t.Run(offset.String(), func(t *testing.T) {
			started := time.Now().Add(offset)
			result := (&runner{version: "test"}).newResult("source.txt", started)
			minimum := max(0, int(time.Since(started).Milliseconds()))
			finishProcessing(&result, started)
			maximum := max(0, int(time.Since(started).Milliseconds()))
			if len(result.Processing) != 1 || result.Processing[0].DurationMs == nil {
				t.Fatal("Не установлена длительность единственного запуска")
			}
			if got := *result.Processing[0].DurationMs; got < minimum || got > maximum {
				t.Fatalf("Длительность %d мс вне диапазона [%d, %d]", got, minimum, maximum)
			}
		})
	}
}

func TestFailedReaderReturnsOriginalErrorWithoutBytes(t *testing.T) {
	cause := &os.PathError{Op: "open", Path: "missing.txt", Err: os.ErrNotExist}
	buffer := []byte("unchanged")
	n, err := (failedReader{err: cause}).Read(buffer)
	if n != 0 || err != cause || string(buffer) != "unchanged" {
		t.Fatalf("Read изменил буфер или ошибку: n=%d, err=%v, buffer=%q", n, err, buffer)
	}
}

func TestProcessFileSavesOrReportsOneFailure(t *testing.T) {
	for _, test := range []struct {
		name, source, status, detail                 string
		existing, replace, missing, technical, saved bool
		errors                                       int
	}{
		{name: "created", source: "@dsl-version 1.2\nText", status: "УСПЕХ: СОЗДАН", saved: true},
		{name: "nonfatal diagnostic saved", source: "@dsl-version 1.2\n@unknown", status: "УСПЕХ: СОЗДАН", saved: true, errors: 1},
		{name: "fatal diagnostic", status: "ОШИБКА: НЕ СОЗДАН", detail: "фатальная ошибка: P013", errors: 1},
		{name: "open failure", missing: true, status: "ОШИБКА: НЕ СОЗДАН", detail: "фатальная ошибка: IO001", errors: 1},
		{name: "replacement denied", source: "@dsl-version 1.2\nText", existing: true, status: "ОШИБКА: НЕ СОЗДАН", detail: "замена"},
		{name: "replacement allowed", source: "@dsl-version 1.2\nText", existing: true, replace: true, status: "УСПЕХ: ЗАМЕНЁН", saved: true},
		{name: "fatal preserves existing JSON", existing: true, replace: true, status: "ОШИБКА: НЕ СОЗДАН", detail: "фатальная ошибка: P013", errors: 1},
		{name: "technical failure", source: "@dsl-version 1.2\nText", technical: true, status: "ОШИБКА: НЕ СОЗДАН", detail: "не удалось разобрать файл"},
		{name: "technical failure preserves JSON", source: "@dsl-version 1.2\nText", technical: true, existing: true, replace: true, status: "ОШИБКА: НЕ СОЗДАН", detail: "не удалось разобрать файл"},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := runnerForTest(t)
			r.params.Replace = test.replace
			path, target := filepath.Join(r.params.Path, "source.TXT"), filepath.Join(r.params.Path, "source.json")
			if !test.missing {
				if err := os.WriteFile(path, []byte(test.source), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if test.existing {
				if err := os.WriteFile(target, []byte("previous JSON"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if test.technical {
				r.diagnostics = nil
			}
			var code int
			before := time.Now().UTC().Truncate(time.Second)
			output := captureRunnerOutput(t, func() { r.processFile(path); code = r.reporter.Finish() })
			lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
			wantCode, success := 1, 0
			if test.saved {
				wantCode, success = 0, 1
			}
			if len(lines) != 2 || code != wantCode || lines[1] != fmt.Sprintf("ИТОГО | всего: 1 | успех: %d | ошибка: %d", success, wantCode) {
				t.Fatalf("Ожидается один отчёт и согласованный итог: %q, код %d", output, code)
			}
			if !strings.HasPrefix(lines[0], test.status+" | ") || !strings.Contains(lines[0], fmt.Sprintf("путь: %q", path)) || !strings.Contains(lines[0], fmt.Sprintf(" | ошибок: %d", test.errors)) || !strings.Contains(lines[0], test.detail) {
				t.Fatalf("Неверный отчёт файла: %q", lines[0])
			}
			payload, err := os.ReadFile(target)
			if !test.saved {
				if test.existing {
					if err != nil || string(payload) != "previous JSON" {
						t.Fatalf("Существующий JSON изменён при отказе: %q, %v", payload, err)
					}
				} else if !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("JSON появился при отказе: %q, %v", payload, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var result model.Result
			if err := json.Unmarshal(payload, &result); err != nil {
				t.Fatal(err)
			}
			if result.Format != model.FormatVersion1 || result.Document.FilePath == nil || *result.Document.FilePath != path || result.Document.FileName == nil || *result.Document.FileName != "source.TXT" || len(result.Lines) != 2 || len(result.Diagnostics) != test.errors || result.Document.HasErrors != (test.errors > 0) {
				t.Fatalf("Неверный сохранённый результат: %+v", result)
			}
			if len(result.Processing) != 1 || result.Processing[0].DurationMs == nil || *result.Processing[0].DurationMs < 0 || result.Processing[0].StartedAt == nil || result.Processing[0].Version == nil || *result.Processing[0].Version != r.version {
				t.Fatalf("В JSON отсутствует завершённая обработка: %+v", result.Processing)
			}
			started, err := time.Parse(time.RFC3339, *result.Processing[0].StartedAt)
			if err != nil || started.Before(before) || started.After(time.Now()) || !strings.HasSuffix(*result.Processing[0].StartedAt, "Z") {
				t.Fatalf("Неверное время начала обработки: %v, %v", started, err)
			}
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Fatal || diagnostic.SeverityLevel != diagnostics.SeverityError || diagnostic.Source != result.Processing[0].ID {
					t.Fatalf("Неверная нефатальная диагностика: %+v", diagnostic)
				}
			}
		})
	}
}

func TestRecordFailureCountsOnlySeverityErrors(t *testing.T) {
	r := runnerForTest(t)
	path := filepath.Join(r.params.Path, "failure.txt")
	result := r.newResult(path, time.Now())
	for _, severity := range []diagnostics.Severity{diagnostics.SeverityError, diagnostics.SeverityWarning, diagnostics.SeverityRecommendation, diagnostics.SeverityError} {
		result.Diagnostics = append(result.Diagnostics, model.Diagnostic{SeverityLevel: severity})
	}
	cause := errors.Join(errors.New("сбой разбора"), errors.New("сбой закрытия"))
	output := captureRunnerOutput(t, func() { r.recordFailure(result, path, cause) })
	want := fmt.Sprintf("ОШИБКА: НЕ СОЗДАН | путь: %q | строк: нет данных | ошибок: 2 | причина: сбой разбора\\nсбой закрытия\n", path)
	if output != want {
		t.Fatalf("Отчёт = %q, требуется %q", output, want)
	}
	entries, err := os.ReadDir(r.params.Path)
	if err != nil || len(entries) != 0 {
		t.Fatalf("Отчёт об отказе изменил каталог: %v, %v", entries, err)
	}
}

func TestProcessFileContinuesWithIndependentResultAfterFailure(t *testing.T) {
	r := runnerForTest(t)
	missing, valid := filepath.Join(r.params.Path, "missing.txt"), filepath.Join(r.params.Path, "valid.txt")
	if err := os.WriteFile(valid, []byte("@dsl-version 1.2\nText"), 0600); err != nil {
		t.Fatal(err)
	}
	var code int
	output := captureRunnerOutput(t, func() { r.processFile(missing); r.processFile(valid); code = r.reporter.Finish() })
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 3 || !strings.Contains(lines[0], "фатальная ошибка: IO001") || !strings.HasPrefix(lines[1], "УСПЕХ: СОЗДАН") || lines[2] != "ИТОГО | всего: 2 | успех: 1 | ошибка: 1" || code != 1 {
		t.Fatalf("Неверное продолжение после отказа: %q, код %d", output, code)
	}
	payload, err := os.ReadFile(filepath.Join(r.params.Path, "valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result model.Result
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Processing) != 1 || result.Document.HasErrors || len(result.Diagnostics) != 0 || len(result.Lines) != 2 || result.Document.FilePath == nil || *result.Document.FilePath != valid {
		t.Fatalf("Следующий файл унаследовал состояние отказа: %+v", result)
	}
}

func runnerForTest(t *testing.T) *runner {
	t.Helper()
	grammars, assertions, err := prepareGrammar()
	if err != nil {
		t.Fatal(err)
	}
	return &runner{params: console.Params{Path: t.TempDir()}, version: "test-version", grammars: grammars,
		diagnostics: diagnostics.NewRegistry(), assertions: assertions, reporter: reporter.New()}
}

func captureRunnerOutput(t *testing.T, run func()) string {
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
			t.Errorf("Закрытие перехвата stdout: %v", err)
		}
	}()
	run()
	payload, err := os.ReadFile(capture.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
