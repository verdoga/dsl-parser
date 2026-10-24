package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/verdoga/dsl-parser/model"
)

// These expectations describe the input files, not values computed by reporter.
// Set DSL_PARSER_BASELINE to a pre-PR17 executable to compare all saved JSON
// fields (except run metadata and the temporary source path) and exit codes.
type cliSource struct {
	name, text, action string
	parseErrors        int
	parseCode          string
	fatalCodes         []string
	previous           bool
	blockedTarget      bool
}

type cliBatch struct {
	name   string
	files  []cliSource
	flags  []string
	totals [5]int // total, saved, failed files, files with nonfatal errors, nonfatal errors
	exit   int
}

func syntheticBatches() []cliBatch {
	const valid = "@dsl-version 1.2\nText\n"
	const nonfatal = "@dsl-version 1.2\n@unknown\n"
	batches := []cliBatch{
		{name: "empty selection", totals: [5]int{}},
		{name: "clean LF CRLF BOM and Unicode", files: []cliSource{
			{name: "a.txt", text: valid, action: "СОЗДАН"},
			{name: "sub/урок.TXT", text: "\ufeff@dsl-version 1.2\r\nПривет!\r\n", action: "СОЗДАН"},
		}, totals: [5]int{2, 2, 0, 0, 0}},
		{name: "multiple nonfatal errors", files: []cliSource{
			{name: "a.txt", text: nonfatal + "@unknown\n@unknown\n", action: "СОЗДАН", parseErrors: 3},
			{name: "b.txt", text: nonfatal, action: "СОЗДАН", parseErrors: 1},
			{name: "clean.txt", text: valid, action: "СОЗДАН"},
		}, totals: [5]int{3, 3, 0, 2, 4}},
		{name: "fatal version errors", files: []cliSource{
			{name: "empty.txt", action: "НЕ СОЗДАН", fatalCodes: []string{"P013"}},
			{name: "missing.txt", text: "Text\n", action: "НЕ СОЗДАН", fatalCodes: []string{"P013"}},
			{name: "unsupported.txt", text: "@dsl-version 9.9\n", action: "НЕ СОЗДАН", fatalCodes: []string{"P014"}},
		}, totals: [5]int{3, 0, 3, 0, 0}, exit: 1},
		{name: "fatal byte errors counted once per file", files: []cliSource{
			{name: "utf8.txt", text: "\xff\n", action: "НЕ СОЗДАН", fatalCodes: []string{"P001"}},
			{name: "cr.txt", text: "@dsl-version 1.2\rText", action: "НЕ СОЗДАН", fatalCodes: []string{"P002"}},
			{name: "both.txt", text: "\xff\r", action: "НЕ СОЗДАН", fatalCodes: []string{"P001", "P002"}},
			{name: "z-valid.txt", text: valid, action: "СОЗДАН"},
		}, totals: [5]int{4, 1, 3, 0, 0}, exit: 1},
		{name: "mixed parse and save failures", files: []cliSource{
			{name: "clean.txt", text: valid, action: "СОЗДАН"},
			{name: "nonfatal.txt", text: nonfatal + "@unknown\n", action: "СОЗДАН", parseErrors: 2},
			{name: "fatal.txt", text: "Text", action: "НЕ СОЗДАН", fatalCodes: []string{"P013"}},
			{name: "denied.txt", text: valid, action: "НЕ СОЗДАН", previous: true},
			{name: "nonfatal-denied.txt", text: nonfatal, action: "НЕ СОЗДАН", previous: true, parseErrors: 1},
			{name: "fatal-denied.txt", text: "Text", action: "НЕ СОЗДАН", previous: true, fatalCodes: []string{"P013"}},
		}, totals: [5]int{6, 2, 4, 2, 3}, exit: 1},
		{name: "replace with fatal preserving previous JSON", flags: []string{"--replace"}, files: []cliSource{
			{name: "clean.txt", text: valid, action: "ЗАМЕНЁН", previous: true},
			{name: "nonfatal.txt", text: nonfatal, action: "ЗАМЕНЁН", previous: true, parseErrors: 1},
			{name: "fatal.txt", text: "Text", action: "НЕ СОЗДАН", previous: true, fatalCodes: []string{"P013"}},
		}, totals: [5]int{3, 2, 1, 1, 1}, exit: 1},
		{name: "directory blocks JSON even with replace", flags: []string{"--replace"}, files: []cliSource{
			{name: "clean.txt", text: valid, action: "НЕ СОЗДАН", blockedTarget: true},
			{name: "nonfatal.txt", text: nonfatal, action: "НЕ СОЗДАН", blockedTarget: true, parseErrors: 1},
			{name: "z-valid.txt", text: valid, action: "СОЗДАН"},
		}, totals: [5]int{3, 1, 2, 1, 1}, exit: 1},
		{name: "depth selection", flags: []string{"--depth", "0"}, files: []cliSource{
			{name: "clean.txt", text: valid, action: "СОЗДАН"},
			{name: "sub/excluded.txt", text: nonfatal},
		}, totals: [5]int{1, 1, 0, 0, 0}},
	}
	batch := cliBatch{name: "user log 121 clean replacements and three P013", flags: []string{"--replace"}, totals: [5]int{124, 121, 3, 0, 0}, exit: 1}
	for i := 0; i < 121; i++ {
		batch.files = append(batch.files, cliSource{name: fmt.Sprintf("lessons/%03d.txt", i), text: valid, previous: true, action: "ЗАМЕНЁН"})
	}
	for _, name := range []string{"VOX Answers.txt", "Videos Answers.txt", "WB Answers.txt"} {
		batch.files = append(batch.files, cliSource{name: name, text: "Text without DSL version\n", action: "НЕ СОЗДАН", fatalCodes: []string{"P013"}})
	}
	return append(batches, batch)
}

func TestCLISyntheticBatches(t *testing.T) {
	binary := buildCLI(t)
	baseline := os.Getenv("DSL_PARSER_BASELINE")
	batches := syntheticBatches()
	for _, fixture := range []struct {
		path   string
		errors int
		code   string
	}{
		{"app/testdata/Work.txt", 2, "P006"},
		{"parser/testdata/multifill-review.txt", 0, ""},
	} {
		payload, err := os.ReadFile(fixture.path)
		if err != nil {
			t.Fatal(err)
		}
		filesWithErrors := 0
		if fixture.errors > 0 {
			filesWithErrors = 1
		}
		batches = append(batches, cliBatch{name: "production fixture " + fixture.path, files: []cliSource{{name: "source.txt", text: string(payload), action: "СОЗДАН", parseErrors: fixture.errors, parseCode: fixture.code}}, totals: [5]int{1, 1, 0, filesWithErrors, fixture.errors}})
	}
	for _, batch := range batches {
		t.Run(batch.name, func(t *testing.T) {
			root := prepareBatch(t, batch)
			stdout, stderr, code := runCLI(t, binary, append(append([]string{}, batch.flags...), root)...)
			lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
			wantTotal := fmt.Sprintf("ИТОГО | всего: %d | сохранено: %d | сбоев: %d | файлов с ошибками разбора: %d | ошибок разбора: %d", batch.totals[0], batch.totals[1], batch.totals[2], batch.totals[3], batch.totals[4])
			if code != batch.exit || stderr != "" || len(lines) != batch.totals[0]+2 || lines[0] != "dsl-parser v"+version || lines[len(lines)-1] != wantTotal {
				t.Fatalf("expected exit=%d, %q; got exit=%d stdout=%s stderr=%s", batch.exit, wantTotal, code, stdout, stderr)
			}
			results := checkBatchFiles(t, batch, root, stdout, true)
			// An independent reconciliation with the visible per-file rows.
			var reconciled [5]int
			for _, line := range lines[1 : len(lines)-1] {
				reconciled[0]++
				if !strings.Contains(line, ": НЕ СОЗДАН |") {
					reconciled[1]++
				}
				if strings.HasPrefix(line, "ОШИБКА:") {
					reconciled[2]++
				}
				_, field, found := strings.Cut(line, " | ошибок разбора: ")
				var count int
				if n, err := fmt.Sscanf(field, "%d", &count); !found || err != nil || n != 1 {
					t.Fatalf("missing parse count: %s", line)
				}
				if count > 0 {
					reconciled[3]++
				}
				reconciled[4] += count
			}
			if reconciled != batch.totals {
				t.Fatalf("file rows %v disagree with expected totals %v", reconciled, batch.totals)
			}
			if baseline != "" {
				oldRoot := prepareBatch(t, batch)
				oldOut, oldErr, oldCode := runCLI(t, baseline, append(append([]string{}, batch.flags...), oldRoot)...)
				if oldCode != code || oldErr != stderr {
					t.Fatalf("pre-PR17 exit/stderr changed: %d %q vs %d %q", oldCode, oldErr, code, stderr)
				}
				oldResults := checkBatchFiles(t, batch, oldRoot, oldOut, false)
				for name, result := range results {
					old := oldResults[name]
					normalizeRunMetadata(&result)
					normalizeRunMetadata(&old)
					if !reflect.DeepEqual(result, old) {
						t.Fatalf("JSON changed since pre-PR17 for %s\nold: %+v\nnew: %+v", name, old, result)
					}
				}
			}
			t.Logf("%v exit=%d; baseline compared=%t", batch.totals, code, baseline != "")
		})
	}
}

func buildCLI(t *testing.T) string {
	t.Helper()
	name := "dslparser"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	command := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", binary, ".")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	return binary
}

func TestCLIFileSystemFailures(t *testing.T) {
	binary := buildCLI(t)
	const valid = "@dsl-version 1.2\nText\n"
	for _, kind := range []string{"read", "write", "walk"} {
		t.Run(kind, func(t *testing.T) {
			batch := cliBatch{flags: []string{"--replace"}, files: []cliSource{{name: "clean.txt", text: valid, action: "СОЗДАН"}}, totals: [5]int{2, 1, 1, 0, 0}, exit: 1}
			switch kind {
			case "read":
				batch.files = append(batch.files, cliSource{name: "unreadable.txt", text: valid, action: "НЕ СОЗДАН", previous: true, fatalCodes: []string{"IO001"}})
			case "write":
				batch.files = append(batch.files, cliSource{name: "denied/nonfatal.txt", text: "@dsl-version 1.2\n@unknown\n", action: "НЕ СОЗДАН", parseErrors: 1})
				batch.totals = [5]int{2, 1, 1, 1, 1}
			case "walk":
				batch.files = append(batch.files, cliSource{name: "denied/hidden.txt", text: valid})
				batch.totals = [5]int{1, 1, 0, 0, 0}
			}
			binaries := []string{binary}
			if baseline := os.Getenv("DSL_PARSER_BASELINE"); baseline != "" {
				binaries = append(binaries, baseline)
			}
			var currentResults map[string]model.Result
			for i, executable := range binaries {
				root := prepareBatch(t, batch)
				denied, mode := filepath.Join(root, "denied"), os.FileMode(0)
				if kind == "read" {
					denied = filepath.Join(root, "unreadable.txt")
				}
				if kind == "write" {
					mode = 0500
				}
				if err := os.Chmod(denied, mode); err != nil {
					t.Fatal(err)
				}
				restore := func() {
					if err := os.Chmod(denied, 0700); err != nil {
						t.Error(err)
					}
				}
				t.Cleanup(restore)
				var probeErr error
				switch kind {
				case "read":
					_, probeErr = os.ReadFile(denied)
				case "write":
					probeErr = os.WriteFile(filepath.Join(denied, "probe"), nil, 0600)
				case "walk":
					_, probeErr = os.ReadDir(denied)
				}
				if !errors.Is(probeErr, os.ErrPermission) {
					t.Skipf("permission refusal cannot be reproduced on this host (%s): %v", kind, probeErr)
				}
				stdout, stderr, code := runCLI(t, executable, "--replace", root)
				restore()
				if code != 1 {
					t.Fatalf("expected failure: code=%d stdout=%s stderr=%s", code, stdout, stderr)
				}
				if (kind == "walk") != strings.Contains(stderr, "Ошибка поиска файлов:") || (kind != "walk" && stderr != "") {
					t.Fatalf("unexpected stderr: %s", stderr)
				}
				if i == 0 {
					want := fmt.Sprintf("ИТОГО | всего: %d | сохранено: %d | сбоев: %d | файлов с ошибками разбора: %d | ошибок разбора: %d", batch.totals[0], batch.totals[1], batch.totals[2], batch.totals[3], batch.totals[4])
					if kind == "walk" {
						want += " | обход: неполный"
					}
					if !strings.HasSuffix(stdout, want+"\n") {
						t.Fatalf("expected %q; got %s", want, stdout)
					}
					currentResults = checkBatchFiles(t, batch, root, stdout, true)
				} else {
					oldResults := checkBatchFiles(t, batch, root, stdout, false)
					for name, result := range currentResults {
						old := oldResults[name]
						normalizeRunMetadata(&old)
						normalizeRunMetadata(&result)
						if !reflect.DeepEqual(old, result) {
							t.Fatalf("JSON changed since pre-PR17: %s", name)
						}
					}
				}
			}
			t.Logf("%v exit=1; baseline compared=%t", batch.totals, len(binaries) == 2)
		})
	}
}

func runCLI(t *testing.T, binary string, args ...string) (string, string, int) {
	t.Helper()
	command := exec.Command(binary, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return stdout.String(), stderr.String(), command.ProcessState.ExitCode()
}

func prepareBatch(t *testing.T, batch cliBatch) string {
	t.Helper()
	root := t.TempDir()
	for _, file := range batch.files {
		path := filepath.Join(root, file.name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(file.text), 0600); err != nil {
			t.Fatal(err)
		}
		target := strings.TrimSuffix(path, filepath.Ext(path)) + ".json"
		if file.previous {
			if err := os.WriteFile(target, []byte("previous JSON"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if file.blockedTarget {
			if err := os.Mkdir(target, 0700); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

func checkBatchFiles(t *testing.T, batch cliBatch, root, stdout string, current bool) map[string]model.Result {
	t.Helper()
	results := make(map[string]model.Result)
	for _, file := range batch.files {
		path := filepath.Join(root, file.name)
		payload, err := os.ReadFile(path)
		if err != nil || string(payload) != file.text {
			t.Fatalf("source changed: %s %v", path, err)
		}
		target := strings.TrimSuffix(path, filepath.Ext(path)) + ".json"
		var row string
		for _, line := range strings.Split(stdout, "\n") {
			if strings.Contains(line, fmt.Sprintf(" | путь: %q |", path)) {
				row = line
				break
			}
		}
		if file.action == "" {
			if row != "" {
				t.Fatalf("excluded source was processed: %s", row)
			}
		} else {
			status := "УСПЕХ"
			if file.action == "НЕ СОЗДАН" {
				status = "ОШИБКА"
			}
			if !strings.HasPrefix(row, status+": "+file.action+" |") {
				t.Fatalf("unexpected file outcome: %s", row)
			}
			if current && !strings.Contains(row+" |", fmt.Sprintf(" | ошибок разбора: %d |", file.parseErrors)) {
				t.Fatalf("unexpected nonfatal errors: %s", row)
			}
			for _, code := range file.fatalCodes {
				if !strings.Contains(row, code) {
					t.Fatalf("missing fatal code %s: %s", code, row)
				}
			}
		}
		if file.blockedTarget {
			info, err := os.Stat(target)
			if err != nil || !info.IsDir() {
				t.Fatalf("target directory changed: %s %v", target, err)
			}
			continue
		}
		payload, err = os.ReadFile(target)
		if file.action == "" || file.action == "НЕ СОЗДАН" {
			if file.previous {
				if err != nil || string(payload) != "previous JSON" {
					t.Fatalf("previous JSON changed: %s", target)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unexpected JSON: %s %v", target, err)
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
		if len(result.Diagnostics) != file.parseErrors || result.Document.HasErrors != (file.parseErrors > 0) {
			t.Fatalf("unexpected diagnostics in %s: %+v", target, result.Diagnostics)
		}
		wantCode := file.parseCode
		if wantCode == "" {
			wantCode = "P003"
		}
		for _, d := range result.Diagnostics {
			if d.Fatal || d.SeverityLevel != "error" || string(d.DiagnosticCode) != wantCode {
				t.Fatalf("unexpected diagnostic: %+v", d)
			}
		}
		if len(result.Processing) != 1 {
			t.Fatalf("unexpected processing: %+v", result.Processing)
		}
		if current {
			p := result.Processing[0]
			if p.Tool != "dsl-parser" || p.Version == nil || *p.Version != version || p.DurationMs == nil || *p.DurationMs < 1 {
				t.Fatalf("incorrect binary metadata: %+v", p)
			}
		}
		results[file.name] = result
	}
	if len(results) != batch.totals[1] {
		t.Fatalf("saved JSON count=%d, expected=%d", len(results), batch.totals[1])
	}
	return results
}

func normalizeRunMetadata(result *model.Result) {
	result.Document.FilePath = nil
	for i := range result.Processing {
		result.Processing[i].Version = nil
		result.Processing[i].StartedAt = nil
		result.Processing[i].DurationMs = nil
	}
}
