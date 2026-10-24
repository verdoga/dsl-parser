package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-parser/console"
	"github.com/verdoga/dsl-parser/model"
)

func TestRunStopsForHelpUsageAndEmptyVersion(t *testing.T) {
	root := t.TempDir()
	path := writeRunSource(t, root, "source.txt", "@dsl-version 1.2\nText")
	var help bytes.Buffer
	if err := console.PrintHelp(&help); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, version string
		args          []string
		code          int
	}{
		{name: "help", args: []string{"--help"}, version: "test"},
		{name: "help before version and path", args: []string{"-h", filepath.Join(root, "missing")}},
		{name: "missing argument", version: "test", code: 2},
		{name: "unknown flag", args: []string{"--unknown", path}, version: "test", code: 2},
		{name: "invalid depth", args: []string{"--depth", "-1", path}, version: "test", code: 2},
		{name: "invalid path before version", args: []string{filepath.Join(root, "missing")}, code: 2},
		{name: "empty version", args: []string{path}, code: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := captureRunOutput(t, func() int { return Run(test.args, test.version) })
			if code != test.code {
				t.Fatalf("Код %d, требуется %d: %q, %q", code, test.code, stdout, stderr)
			}
			banner := ""
			if test.version != "" {
				banner = "dsl-parser v" + test.version + "\n"
			}
			switch test.code {
			case 0:
				if stdout != banner+help.String() || stderr != "" {
					t.Fatalf("Неверный вывод справки: %q, %q", stdout, stderr)
				}
			case 2:
				var wantOut, wantErr bytes.Buffer
				console.Parse(test.args, &wantOut, &wantErr)
				if stdout != banner+wantOut.String() || stderr != wantErr.String() || strings.Count(stderr, "\n") != 1 {
					t.Fatalf("Ошибка console изменена или напечатана повторно: %q, %q", stdout, stderr)
				}
			case 1:
				if stdout != "" || !strings.Contains(stderr, "версия инструмента пуста") || strings.Count(stderr, "\n") != 1 {
					t.Fatalf("Неверная ошибка конфигурации: %q, %q", stdout, stderr)
				}
			}
			if _, err := os.Stat(filepath.Join(root, "source.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("Обработка началась после остановки запуска: %v", err)
			}
		})
	}
}

func TestRunReturnsUsageWhenHelpCannotBeWritten(t *testing.T) {
	closed, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := captureRunOutput(t, func() int {
		previous := os.Stdout
		os.Stdout = closed
		defer func() { os.Stdout = previous }()
		return Run([]string{"--help"}, "")
	})
	if code != 2 || stdout != "" || stderr != "" {
		t.Fatalf("Ошибка вывода справки: код %d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
}

func TestRunProcessesSelectedFilesAndReportsOutcomes(t *testing.T) {
	valid := "@dsl-version 1.2\nText"
	for _, test := range []struct {
		name, source                                         string
		files                                                map[string]string
		flags                                                []string
		selected                                             []string
		statuses                                             []string
		existing                                             bool
		technicalFailures, filesWithParseErrors, parseErrors int
	}{
		{name: "empty directory"},
		{name: "single file ignores depth", source: "a.txt", files: map[string]string{"a.txt": valid}, flags: []string{"--depth", "0"}, selected: []string{"a.txt"}, statuses: []string{"УСПЕХ: СОЗДАН"}},
		{name: "sorted recursive selection", files: map[string]string{"z.txt": valid, "a.TXT": valid, "sub/b.txt": valid, "ignored.md": valid}, selected: []string{"a.TXT", "sub/b.txt", "z.txt"}, statuses: []string{"УСПЕХ: СОЗДАН", "УСПЕХ: СОЗДАН", "УСПЕХ: СОЗДАН"}},
		{name: "depth zero", files: map[string]string{"a.txt": valid, "sub/b.txt": valid}, flags: []string{"--depth", "0"}, selected: []string{"a.txt"}, statuses: []string{"УСПЕХ: СОЗДАН"}},
		{name: "depth one", files: map[string]string{"a.txt": valid, "sub/b.txt": valid, "sub/deep/c.txt": valid}, flags: []string{"--depth", "1"}, selected: []string{"a.txt", "sub/b.txt"}, statuses: []string{"УСПЕХ: СОЗДАН", "УСПЕХ: СОЗДАН"}},
		{name: "nonfatal errors still succeed", files: map[string]string{"a.txt": "@dsl-version 1.2\n@unknown"}, selected: []string{"a.txt"}, statuses: []string{"УСПЕХ: СОЗДАН"}, filesWithParseErrors: 1, parseErrors: 1},
		{name: "continue after fatal error", files: map[string]string{"a.txt": "", "z.txt": valid}, selected: []string{"a.txt", "z.txt"}, statuses: []string{"ОШИБКА: НЕ СОЗДАН", "УСПЕХ: СОЗДАН"}, filesWithParseErrors: 1, parseErrors: 1},
		{name: "replacement denied", files: map[string]string{"a.txt": valid}, existing: true, selected: []string{"a.txt"}, statuses: []string{"ОШИБКА: НЕ СОЗДАН"}, technicalFailures: 1},
		{name: "replacement allowed", files: map[string]string{"a.txt": valid}, existing: true, flags: []string{"--replace"}, selected: []string{"a.txt"}, statuses: []string{"УСПЕХ: ЗАМЕНЁН"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for name, source := range test.files {
				writeRunSource(t, root, name, source)
			}
			if test.existing {
				writeRunSource(t, root, "a.json", "previous JSON")
			}
			args := append(append([]string{}, test.flags...), filepath.Join(root, test.source))
			code, stdout, stderr := captureRunOutput(t, func() int { return Run(args, "build-test") })
			lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
			if stderr != "" || len(lines) != len(test.selected)+2 || lines[0] != "dsl-parser vbuild-test" {
				t.Fatalf("Неверное число отчётов или лишний stderr: %q, %q", stdout, stderr)
			}
			banner := lines[0]
			lines = lines[1:]
			written := make(map[string]bool)
			failures := 0
			for i, name := range test.selected {
				if !strings.HasPrefix(lines[i], fmt.Sprintf("%s | путь: %q | ", test.statuses[i], filepath.Join(root, name))) {
					t.Fatalf("Неверный порядок или статус файла %s: %q", name, lines[i])
				}
				if strings.HasPrefix(test.statuses[i], "ОШИБКА:") {
					failures++
				} else {
					written[name] = true
				}
			}
			wantCode := 0
			if failures != 0 {
				wantCode = 1
			}
			wantTotal := fmt.Sprintf("ИТОГО | всего: %d | сохранено: %d | сбоев: %d | файлов с ошибками разбора: %d | ошибок разбора: %d",
				len(test.selected), len(written), test.technicalFailures, test.filesWithParseErrors, test.parseErrors)
			if code != wantCode || lines[len(lines)-1] != wantTotal {
				t.Fatalf("Неверный итог: код %d, %q; требуется %d, %q", code, stdout, wantCode, wantTotal)
			}
			for name, source := range test.files {
				target := filepath.Join(root, strings.TrimSuffix(name, filepath.Ext(name))+".json")
				payload, err := os.ReadFile(target)
				if !written[name] {
					if test.existing && name == "a.txt" {
						if err != nil || string(payload) != "previous JSON" {
							t.Fatalf("JSON изменён без разрешения: %q, %v", payload, err)
						}
					} else if !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("Неожиданный JSON для %s: %q, %v", name, payload, err)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				var result model.Result
				if err := json.Unmarshal(payload, &result); err != nil {
					t.Fatal(err)
				}
				if len(result.Processing) != 1 || result.Processing[0].Version == nil || *result.Processing[0].Version != "build-test" || result.Document.HasErrors != (source != valid) {
					t.Fatalf("Версия инструмента или ошибки DSL потеряны: %+v", result)
				}
				if banner != fmt.Sprintf("%s v%s", result.Processing[0].Tool, *result.Processing[0].Version) {
					t.Fatalf("Заголовок %q не совпадает с processing: %+v", banner, result.Processing[0])
				}
			}
		})
	}
}

func TestRunPreservesPartialDiscoveryResultsAndFailsOnWalkError(t *testing.T) {
	for _, count := range []int{0, 2} {
		t.Run(fmt.Sprintf("accessible files %d", count), func(t *testing.T) {
			root := t.TempDir()
			denied := filepath.Join(root, "a-denied")
			writeRunSource(t, denied, "hidden.txt", "@dsl-version 1.2")
			if err := os.Chmod(denied, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chmod(denied, 0700); err != nil {
					t.Error(err)
				}
			})
			if _, err := os.ReadDir(denied); !errors.Is(err, os.ErrPermission) {
				t.Skipf("Среда не позволяет воспроизвести отказ доступа: %v", err)
			}
			for i := 0; i < count; i++ {
				writeRunSource(t, root, fmt.Sprintf("file%d.txt", i), "@dsl-version 1.2")
			}
			code, stdout, stderr := captureRunOutput(t, func() int { return Run([]string{root}, "test") })
			wantTotal := fmt.Sprintf("ИТОГО | всего: %d | сохранено: %d | сбоев: 0 | файлов с ошибками разбора: 0 | ошибок разбора: 0\n", count, count)
			if code != 1 || !strings.Contains(stderr, denied) || strings.Count(stderr, "\n") != 1 || !strings.HasSuffix(stdout, wantTotal) || strings.Count(stdout, "\n") != count+2 {
				t.Fatalf("Ошибка обхода или частичный результат потеряны: код %d, %q, %q", code, stdout, stderr)
			}
			for i := 0; i < count; i++ {
				if _, err := os.Stat(filepath.Join(root, fmt.Sprintf("file%d.json", i))); err != nil {
					t.Fatalf("Доступный источник не обработан: %v", err)
				}
			}
		})
	}
}

func TestRunReportsBatchCountsAndDurations(t *testing.T) {
	for _, test := range []struct {
		name         string
		count        int
		errorsByFile map[int]int
		wantTotal    string
	}{
		{"provided example", 14, map[int]int{0: 1, 6: 1}, "ИТОГО | всего: 14 | сохранено: 14 | сбоев: 0 | файлов с ошибками разбора: 2 | ошибок разбора: 2"},
		{"large queue", 120, map[int]int{1: 2, 61: 1, 119: 1}, "ИТОГО | всего: 120 | сохранено: 120 | сбоев: 0 | файлов с ошибками разбора: 3 | ошибок разбора: 4"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			for i := 0; i < test.count; i++ {
				source := "@dsl-version 1.2\n" + strings.Repeat("Example text.\n", 259-test.errorsByFile[i]) + strings.Repeat("@unknown\n", test.errorsByFile[i])
				writeRunSource(t, root, fmt.Sprintf("%03d.txt", i), source)
			}
			code, stdout, stderr := captureRunOutput(t, func() int { return Run([]string{root}, "1.0.3") })
			lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
			if code != 0 || stderr != "" || len(lines) != test.count+2 || lines[0] != "dsl-parser v1.0.3" || lines[len(lines)-1] != test.wantTotal {
				t.Fatalf("Неверный пакетный отчёт: код %d, stdout=%q, stderr=%q", code, stdout, stderr)
			}
			for i := 0; i < test.count; i++ {
				path := filepath.Join(root, fmt.Sprintf("%03d.txt", i))
				wantLine := fmt.Sprintf("УСПЕХ: СОЗДАН | путь: %q | строк: 260 | ошибок: %d", path, test.errorsByFile[i])
				if lines[i+1] != wantLine {
					t.Fatalf("Строка файла %d: %q, требуется %q", i, lines[i+1], wantLine)
				}
				payload, err := os.ReadFile(filepath.Join(root, fmt.Sprintf("%03d.json", i)))
				if err != nil {
					t.Fatal(err)
				}
				var result model.Result
				if err := json.Unmarshal(payload, &result); err != nil {
					t.Fatal(err)
				}
				if len(result.Processing) != 1 || len(result.Diagnostics) != test.errorsByFile[i] || result.Document.HasErrors != (test.errorsByFile[i] > 0) {
					t.Fatalf("Файл %d унаследовал чужие ошибки или обработку: %+v", i, result)
				}
				processing := result.Processing[0]
				if processing.Version == nil || lines[0] != fmt.Sprintf("%s v%s", processing.Tool, *processing.Version) || processing.DurationMs == nil || *processing.DurationMs < 1 {
					t.Fatalf("Файл %d: неверные версия или длительность: %+v", i, processing)
				}
			}
		})
	}
}

func TestRunCountsSimultaneousParseAndSaveFailures(t *testing.T) {
	root := t.TempDir()
	for _, file := range []struct {
		name, source string
	}{
		{"001-valid", "@dsl-version 1.2\nText"},
		{"002-nonfatal", "@dsl-version 1.2\n@unknown\n@unknown"},
		{"003-fatal", "Text"},
		{"004-denied", "@dsl-version 1.2\nText"},
		{"005-nonfatal-denied", "@dsl-version 1.2\n@unknown"},
		{"006-fatal-denied", "Text"},
	} {
		writeRunSource(t, root, file.name+".txt", file.source)
	}
	for _, name := range []string{"004-denied", "005-nonfatal-denied", "006-fatal-denied"} {
		writeRunSource(t, root, name+".json", "previous JSON")
	}
	code, stdout, stderr := captureRunOutput(t, func() int { return Run([]string{root}, "1.0.3") })
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	wantTotal := "ИТОГО | всего: 6 | сохранено: 2 | сбоев: 3 | файлов с ошибками разбора: 4 | ошибок разбора: 5"
	if code != 1 || stderr != "" || len(lines) != 8 || lines[0] != "dsl-parser v1.0.3" || lines[len(lines)-1] != wantTotal || strings.Count(stdout, "фатальная ошибка: P013") != 2 {
		t.Fatalf("Неверный смешанный отчёт: код %d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
	for _, name := range []string{"004-denied", "005-nonfatal-denied", "006-fatal-denied"} {
		payload, err := os.ReadFile(filepath.Join(root, name+".json"))
		if err != nil || string(payload) != "previous JSON" {
			t.Fatalf("JSON %s изменён при отказе: %q, %v", name, payload, err)
		}
	}
}

func writeRunSource(t *testing.T, root, name, source string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func captureRunOutput(t *testing.T, run func() int) (code int, stdout, stderr string) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	errOut, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer errOut.Close()
	previousOut, previousErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = out, errOut
	defer func() { os.Stdout, os.Stderr = previousOut, previousErr }()
	code = run()
	output, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	errorOutput, err := os.ReadFile(errOut.Name())
	if err != nil {
		t.Fatal(err)
	}
	return code, string(output), string(errorOutput)
}
