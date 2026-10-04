package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verdoga/dsl-parser/console"
	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
)

func TestOutputPathValidatesWithoutCorrectingModel(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "lesson.txt")
	empty, relative := "", "lesson.txt"
	dirty := directory + string(filepath.Separator) + "." + string(filepath.Separator) + "lesson.txt"
	type pathCase struct {
		name, fileName string
		path           *string
		wantSource     string
		jsonName       string
	}
	tests := []pathCase{
		{name: "unknown path"},
		{name: "empty path", path: &empty},
		{name: "relative path", path: &relative},
		{name: "unclean path", path: &dirty},
		{name: "empty name", path: &path, wantSource: path},
		{name: "mismatched name", fileName: "other.txt", path: &path, wantSource: path},
	}
	for name, jsonName := range map[string]string{"lesson.txt": "lesson.json", "lesson.TXT": "lesson.json", "lesson.TxT": "lesson.json", "lesson.part.txt": "lesson.part.json", "lesson.txt.bak": "", "lesson": ""} {
		p := filepath.Join(directory, name)
		tests = append(tests, pathCase{name, name, &p, p, jsonName})
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := &model.Result{Document: model.Document{FilePath: test.path, FileName: &test.fileName}}
			before := storageModelSnapshot(t, result)
			source, target, err := outputPath(result)
			wantTarget := ""
			if test.jsonName != "" {
				wantTarget = filepath.Join(directory, test.jsonName)
			}
			if source != test.wantSource || target != wantTarget || (err == nil) != (test.jsonName != "") {
				t.Fatalf("outputPath = (%q, %q, %v), требуется (%q, %q)", source, target, err, test.wantSource, wantTarget)
			}
			if !bytes.Equal(before, storageModelSnapshot(t, result)) {
				t.Fatal("модель изменена")
			}
		})
	}
	if source, target, err := outputPath(nil); source != "" || target != "" || err == nil {
		t.Fatalf("outputPath(nil) = (%q, %q, %v)", source, target, err)
	}
	if source, target, err := outputPath(&model.Result{Document: model.Document{FilePath: &path}}); source != path || target != "" || err == nil {
		t.Fatalf("неизвестное имя: (%q, %q, %v)", source, target, err)
	}
	assertStorageTestEntries(t, directory)
}

func TestCheckTargetDistinguishesFilesystemObjects(t *testing.T) {
	for _, kind := range []string{"missing", "regular", "directory", "symlink", "dangling", "file parent"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			target, original := filepath.Join(directory, "target.json"), filepath.Join(directory, "original")
			writeStorageTestFile(t, original, []byte("original"))
			switch kind {
			case "regular":
				writeStorageTestFile(t, target, []byte("old JSON"))
			case "directory":
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink", "dangling":
				referent := original
				if kind == "dangling" {
					referent += ".missing"
				}
				if err := os.Symlink(referent, target); err != nil {
					t.Fatal(err)
				}
			case "file parent":
				target = filepath.Join(original, "target.json")
			}
			for _, replace := range []bool{false, true} {
				exists, err := checkTarget(target, replace)
				wantError := kind != "missing" && !(kind == "regular" && replace)
				if exists != (kind == "regular") || (err != nil) != wantError {
					t.Fatalf("checkTarget(%t) = (%t, %v)", replace, exists, err)
				}
				if kind == "file parent" {
					var pathErr *os.PathError
					if !errors.As(err, &pathErr) {
						t.Fatalf("потеряна ошибка Lstat: %v", err)
					}
				}
				if kind == "directory" || kind == "symlink" || kind == "dangling" {
					before, statErr := os.Lstat(target)
					if statErr != nil {
						t.Fatal(statErr)
					}
					source, name := filepath.Join(directory, "target.txt"), "target.txt"
					result := &model.Result{Format: model.FormatVersion1, Document: model.Document{FilePath: &source, FileName: &name}, Diagnostics: []model.Diagnostic{{Fatal: true, Message: "FATAL"}}}
					report := Save(result, console.Params{Replace: replace})
					if report.Action != ActionNotWritten || report.Err == nil || strings.Contains(report.Err.Error(), "FATAL") {
						t.Fatalf("Save не отказал на проверке цели: %+v", report)
					}
					after, statErr := os.Lstat(target)
					if statErr != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
						t.Fatalf("целевой объект изменён: %v", statErr)
					}
					assertStorageTestEntries(t, directory, "original", "target.json")
				}
			}
			assertStorageTestContent(t, original, []byte("original"))
			if kind == "regular" {
				assertStorageTestContent(t, target, []byte("old JSON"))
			}
		})
	}
}

func TestEncodeChecksVersionAndPreservesJSONContract(t *testing.T) {
	for _, result := range []*model.Result{nil, {}, {Format: "2.0"}} {
		if payload, err := encode(result); err == nil || payload != nil {
			t.Fatalf("некорректная модель сериализована: %q, %v", payload, err)
		}
	}
	title := "Урок 🌍 <>&"
	result := &model.Result{Format: model.FormatVersion1, Document: model.Document{Metadata: model.DocumentMetadata{Title: &title, ResourceDirs: []string{}}}, Lines: []model.Line{}}
	want := `{
  "formatVersion": "1.0",
  "document": {
    "dslVersion": null,
    "fileName": null,
    "filePath": null,
    "encoding": null,
    "hasBom": null,
    "lineCount": null,
    "byteLength": null,
    "sha256": null,
    "metadata": {
      "documentId": null,
      "title": "Урок 🌍 <>&",
      "subtitle": null,
      "section": null,
      "order": null,
      "resourceDirs": []
    },
    "hasErrors": false
  },
  "processing": null,
  "lines": [],
  "diagnostics": null
}
`
	payload, err := encode(result)
	if err != nil || string(payload) != want {
		t.Fatalf("JSON = %s, ошибка: %v; требуется %s", payload, err, want)
	}
	result.Processing = []model.Processing{{ID: "z"}, {ID: "a"}}
	result.Lines = []model.Line{{Number: 2, Elements: []model.Element{{ErrorIDs: []string{"z", "a"}}}}, {Number: 1}}
	result.Diagnostics = []model.Diagnostic{{ID: "z", RelatedLocations: []model.Location{}}, {ID: "a"}}
	before := storageModelSnapshot(t, result)
	payload, err = encode(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded model.Result
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, storageModelSnapshot(t, &decoded)) || !bytes.Equal(before, storageModelSnapshot(t, result)) {
		t.Fatal("изменены значения модели, порядок массивов или различие null и []")
	}
}

func TestSaveReportsOutcomeAndPreservesModel(t *testing.T) {
	capture, err := os.CreateTemp(t.TempDir(), "console")
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = capture, capture
	defer func() { os.Stdout, os.Stderr = stdout, stderr; capture.Close() }()
	tests := []struct {
		name, failure                        string
		existing, replace, fatal, badVersion bool
	}{
		{name: "created"},
		{name: "without errors"},
		{name: "warnings only"},
		{name: "created with replace", replace: true},
		{name: "replaced", existing: true, replace: true},
		{name: "replacement denied", failure: "замена", existing: true, fatal: true, badVersion: true},
		{name: "path first", failure: "абсолютным", existing: true, fatal: true, badVersion: true},
		{name: "fatal before encoding", failure: "Z001: первая", fatal: true, badVersion: true},
		{name: "fatal keeps original", failure: "Z001: первая", existing: true, replace: true, fatal: true},
		{name: "unsupported format", failure: "формат", existing: true, replace: true, badVersion: true},
		{name: "missing parent", failure: "временный файл"},
		{name: "nil model", failure: "модель"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			source, name := filepath.Join(directory, "lesson.part.TXT"), "lesson.part.TXT"
			target := filepath.Join(directory, "lesson.part.json")
			result := &model.Result{Format: model.FormatVersion1, Document: model.Document{FilePath: &source, FileName: &name}, Diagnostics: []model.Diagnostic{
				{DiagnosticCode: "Z001", Message: "первая", SeverityLevel: diagnostics.SeverityError, Fatal: test.fatal},
				{DiagnosticCode: "W001", Message: "предупреждение", SeverityLevel: diagnostics.SeverityWarning},
				{DiagnosticCode: "R001", Message: "рекомендация", SeverityLevel: diagnostics.SeverityRecommendation},
				{DiagnosticCode: "A001", Message: "вторая", SeverityLevel: diagnostics.SeverityError, Fatal: test.fatal},
			}}
			if test.existing {
				writeStorageTestFile(t, target, []byte("original"))
			}
			if test.badVersion {
				result.Format = "2.0"
			}
			wantSource, wantTarget, count := source, target, 2
			switch test.name {
			case "without errors":
				result.Diagnostics, count = nil, 0
				result.Document.HasErrors = true
			case "warnings only":
				result.Diagnostics, count = result.Diagnostics[1:3], 0
			case "path first":
				source, wantSource, wantTarget = "relative.txt", "", ""
			case "missing parent":
				source = filepath.Join(directory, "missing", name)
				wantSource, wantTarget = source, filepath.Join(directory, "missing", "lesson.part.json")
			case "nil model":
				result, wantSource, wantTarget, count = nil, "", "", 0
			}
			before := storageModelSnapshot(t, result)
			report := Save(result, console.Params{Path: directory, Replace: test.replace})
			if report.SourcePath != wantSource || report.TargetPath != wantTarget || report.ErrorCount != count {
				t.Fatalf("неверные пути или число ошибок: %+v", report)
			}
			if !bytes.Equal(before, storageModelSnapshot(t, result)) {
				t.Fatal("Save изменил модель")
			}
			if test.failure != "" {
				if report.Action != ActionNotWritten || report.Err == nil || !strings.Contains(report.Err.Error(), test.failure) {
					t.Fatalf("неверный отказ: %+v, ожидается %q", report, test.failure)
				}
				if strings.HasPrefix(test.failure, "Z001") {
					message := report.Err.Error()
					if strings.Index(message, "A001: вторая") <= strings.Index(message, "Z001: первая") || strings.Contains(message, "W001") || strings.Contains(message, "R001") {
						t.Fatalf("неверный набор или порядок фатальных диагностик: %s", message)
					}
				}
				if test.existing {
					assertStorageTestContent(t, target, []byte("original"))
					assertStorageTestEntries(t, directory, "lesson.part.json")
				} else {
					assertStorageTestEntries(t, directory)
				}
				return
			}
			wantAction := ActionCreated
			if test.existing {
				wantAction = ActionReplaced
			}
			if report.Err != nil || report.Action != wantAction {
				t.Fatalf("неверный итог: %+v", report)
			}
			payload, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			var decoded model.Result
			if err := json.Unmarshal(payload, &decoded); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, storageModelSnapshot(t, &decoded)) {
				t.Fatal("записана другая модель")
			}
			assertStorageTestEntries(t, directory, "lesson.part.json")
		})
	}
	if info, err := capture.Stat(); err != nil || info.Size() != 0 {
		t.Fatalf("Save вывел сообщения: %v", err)
	}
}

func storageModelSnapshot(t *testing.T, result *model.Result) []byte {
	t.Helper()
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
