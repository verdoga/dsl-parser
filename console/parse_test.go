package console

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestParseDepth(t *testing.T) {
	overflow := "1" + strings.Repeat("0", strconv.IntSize)
	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{name: "zero", raw: "0"},
		{name: "positive", raw: "12", want: 12},
		{name: "plus", raw: "+3", want: 3},
		{name: "leading zeroes", raw: "003", want: 3},
		{name: "negative", raw: "-1", wantErr: true},
		{name: "empty", wantErr: true},
		{name: "spaces", raw: " 3 ", wantErr: true},
		{name: "fraction", raw: "1.5", wantErr: true},
		{name: "text", raw: "many", wantErr: true},
		{name: "overflow", raw: overflow, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseDepth(test.raw)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseDepth(%q) error = %v, наличие ошибки требуется %t", test.raw, err, test.wantErr)
			}
			if got != test.want {
				t.Errorf("parseDepth(%q) = %d, требуется %d", test.raw, got, test.want)
			}
		})
	}
}

func TestNormalizePath(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("не удалось получить рабочий каталог: %v", err)
	}
	separator := string(os.PathSeparator)
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "relative", raw: filepath.Join("missing", "document.txt"), want: filepath.Join(cwd, "missing", "document.txt")},
		{name: "clean relative", raw: "missing" + separator + "." + separator + "child" + separator + ".." + separator + "document.txt", want: filepath.Join(cwd, "missing", "document.txt")},
		{name: "clean absolute", raw: filepath.Join(cwd, "missing") + separator + "." + separator + "child" + separator + ".." + separator + "document.txt", want: filepath.Join(cwd, "missing", "document.txt")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizePath(test.raw)
			if err != nil {
				t.Fatalf("normalizePath(%q) вернул ошибку: %v", test.raw, err)
			}
			if got != test.want {
				t.Errorf("normalizePath(%q) = %q, требуется %q", test.raw, got, test.want)
			}
		})
	}
}

func TestValidateSourcePath(t *testing.T) {
	dir := t.TempDir()
	textFile := writeTestFile(t, dir, "document.txt")
	upperFile := writeTestFile(t, dir, "document.TXT")
	mixedFile := writeTestFile(t, dir, "document.TxT")
	otherFile := writeTestFile(t, dir, "document.json")
	noExtension := writeTestFile(t, dir, "document")

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{name: "directory", path: dir},
		{name: "txt", path: textFile},
		{name: "uppercase txt", path: upperFile},
		{name: "mixed case txt", path: mixedFile},
		{name: "other extension", path: otherFile, wantErr: true},
		{name: "without extension", path: noExtension, wantErr: true},
		{name: "missing", path: filepath.Join(dir, "missing.txt"), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSourcePath(test.path)
			if (err != nil) != test.wantErr {
				t.Errorf("validateSourcePath(%q) error = %v, наличие ошибки требуется %t", test.path, err, test.wantErr)
			}
		})
	}
}

func TestValidateSourcePathSymlinks(t *testing.T) {
	dir := t.TempDir()
	targetFile := writeTestFile(t, dir, "target.txt")
	targetDir := filepath.Join(dir, "target")
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		t.Fatalf("не удалось создать каталог: %v", err)
	}
	intermediateTarget := writeTestFile(t, targetDir, "nested.txt")
	fileLink := filepath.Join(dir, "file.txt")
	dirLink := filepath.Join(dir, "directory")
	if err := os.Symlink(targetFile, fileLink); err != nil {
		t.Skipf("символические ссылки недоступны: %v", err)
	}
	if err := os.Symlink(targetDir, dirLink); err != nil {
		t.Fatalf("не удалось создать ссылку на каталог: %v", err)
	}

	if err := validateSourcePath(fileLink); err == nil {
		t.Error("validateSourcePath() принял конечную ссылку на файл")
	}
	if err := validateSourcePath(dirLink); err == nil {
		t.Error("validateSourcePath() принял конечную ссылку на каталог")
	}
	if err := validateSourcePath(filepath.Join(dirLink, filepath.Base(intermediateTarget))); err != nil {
		t.Errorf("validateSourcePath() отклонил ссылку в промежуточном компоненте: %v", err)
	}
}

func TestValidateSourcePathRejectsSpecialFile(t *testing.T) {
	info, err := os.Lstat(os.DevNull)
	if err != nil || info.Mode().IsRegular() || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Skip("именованный специальный файл недоступен на этой платформе")
	}
	if err := validateSourcePath(os.DevNull); err == nil {
		t.Errorf("validateSourcePath(%q) принял специальный файл", os.DevNull)
	}
}

func TestParseFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantPath    string
		wantDepth   int
		wantDepthOK bool
		wantReplace bool
		wantHelp    bool
		wantErr     bool
	}{
		{name: "path", args: []string{"source"}, wantPath: "source"},
		{name: "replace", args: []string{"--replace", "source"}, wantPath: "source", wantReplace: true},
		{name: "single dash replace", args: []string{"-replace", "source"}, wantPath: "source", wantReplace: true},
		{name: "depth", args: []string{"--depth", "2", "source"}, wantPath: "source", wantDepth: 2, wantDepthOK: true},
		{name: "depth equals", args: []string{"--depth=3", "source"}, wantPath: "source", wantDepth: 3, wantDepthOK: true},
		{name: "single dash depth", args: []string{"-depth", "0", "source"}, wantPath: "source", wantDepthOK: true},
		{name: "help", args: []string{"--help"}, wantHelp: true},
		{name: "short help", args: []string{"-h"}, wantHelp: true},
		{name: "both help forms", args: []string{"--help", "-h"}, wantHelp: true},
		{name: "help ignores paths", args: []string{"--help", "one", "two"}, wantHelp: true},
		{name: "missing path", wantErr: true},
		{name: "extra path", args: []string{"one", "two"}, wantErr: true},
		{name: "unknown flag", args: []string{"--unknown", "source"}, wantErr: true},
		{name: "invalid depth", args: []string{"--depth", "bad", "source"}, wantErr: true},
		{name: "flag after path", args: []string{"source", "--replace"}, wantErr: true},
		{name: "error before help", args: []string{"--unknown", "--help"}, wantErr: true},
		{name: "error after help", args: []string{"--help", "--unknown"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params, help, err := parseFlags(test.args)
			if (err != nil) != test.wantErr {
				t.Fatalf("parseFlags(%q) error = %v, наличие ошибки требуется %t", test.args, err, test.wantErr)
			}
			if params.Path != test.wantPath || params.Replace != test.wantReplace || help != test.wantHelp {
				t.Errorf("parseFlags(%q) = (%+v, %t), требуется Path=%q Replace=%t help=%t", test.args, params, help, test.wantPath, test.wantReplace, test.wantHelp)
			}
			if (params.Depth != nil) != test.wantDepthOK {
				t.Fatalf("наличие Depth = %t, требуется %t", params.Depth != nil, test.wantDepthOK)
			}
			if params.Depth != nil && *params.Depth != test.wantDepth {
				t.Errorf("Depth = %d, требуется %d", *params.Depth, test.wantDepth)
			}
		})
	}

	first, _, _ := parseFlags([]string{"--replace", "first"})
	second, _, _ := parseFlags([]string{"second"})
	if !first.Replace || second.Replace || second.Depth != nil {
		t.Errorf("состояние независимых разборов связано: first=%+v second=%+v", first, second)
	}
}

func TestParse(t *testing.T) {
	dir := t.TempDir()
	file := writeTestFile(t, dir, "document.TXT")
	tests := []struct {
		name        string
		args        []string
		wantReplace bool
		wantDepth   int
		wantDepthOK bool
	}{
		{name: "directory", args: []string{dir}},
		{name: "file with options", args: []string{"--replace", "--depth", "2", file}, wantReplace: true, wantDepth: 2, wantDepthOK: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			params, proceed, err := Parse(test.args, &stdout, &stderr)
			if err != nil || !proceed {
				t.Fatalf("Parse(%q) = (%+v, %t, %v), требуется успешное продолжение", test.args, params, proceed, err)
			}
			if !filepath.IsAbs(params.Path) || params.Path != filepath.Clean(test.args[len(test.args)-1]) {
				t.Errorf("Path = %q, требуется абсолютный очищенный путь", params.Path)
			}
			if params.Replace != test.wantReplace || (params.Depth != nil) != test.wantDepthOK || (params.Depth != nil && *params.Depth != test.wantDepth) {
				t.Errorf("Params = %+v, требуются Replace=%t Depth=%d", params, test.wantReplace, test.wantDepth)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Errorf("успешный Parse() вывел stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestParseHelpAndErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	_, proceed, err := Parse([]string{"--help"}, &stdout, &stderr)
	if err != nil || proceed || !strings.Contains(stdout.String(), "Использование:") || stderr.Len() != 0 {
		t.Errorf("Parse(--help) = proceed=%t error=%v stdout=%q stderr=%q", proceed, err, stdout.String(), stderr.String())
	}

	dir := t.TempDir()
	wrongExtension := writeTestFile(t, dir, "document.json")
	tests := []struct {
		name string
		args []string
	}{
		{name: "arguments"},
		{name: "missing path", args: []string{filepath.Join(dir, "missing.txt")}},
		{name: "wrong extension", args: []string{wrongExtension}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			_, proceed, err := Parse(test.args, &stdout, &stderr)
			if err == nil || proceed || strings.Count(stderr.String(), "Ошибка:") != 1 {
				t.Errorf("Parse(%q) = proceed=%t error=%v stderr=%q", test.args, proceed, err, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("Parse(%q) вывел stdout=%q при ошибке", test.args, stdout.String())
			}
		})
	}
}

func TestParseReturnsOutputErrors(t *testing.T) {
	writeErr := errors.New("запись недоступна")
	_, proceed, err := Parse([]string{"--help"}, errorWriter{err: writeErr}, &bytes.Buffer{})
	if proceed || !errors.Is(err, writeErr) {
		t.Errorf("ошибка справки = %v, proceed=%t; требуется ошибка writer", err, proceed)
	}

	_, proceed, err = Parse(nil, &bytes.Buffer{}, errorWriter{err: writeErr})
	if proceed || !errors.Is(err, writeErr) || !strings.Contains(err.Error(), "требуется ровно один исходный путь") {
		t.Errorf("ошибка stderr = %v, proceed=%t; требуется ошибка writer с исходным контекстом", err, proceed)
	}
}

func writeTestFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("не удалось создать тестовый файл: %v", err)
	}
	return path
}
