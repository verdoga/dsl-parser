package console

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRejectsSymlinksBeforeCleaning(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Mkdir("target", 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, "target", "source.txt")
	writeTestFile(t, ".", "source.txt")
	for _, link := range []struct{ name, target string }{
		{"link", "target"},
		{"file.txt", "source.txt"},
		{"dangling", "missing"},
	} {
		if err := os.Symlink(link.target, link.name); err != nil {
			t.Fatalf("не удалось создать тестовую ссылку: %v", err)
		}
	}

	paths := []string{
		"link",
		"link/source.txt",
		"link/../source.txt",
		"link/../target/source.txt",
		"target/../link/source.txt",
		"link/.",
		"link/",
		"file.txt",
		"file.txt/../source.txt",
		"dangling/../source.txt",
	}
	for _, raw := range paths {
		for _, path := range []string{raw, root + string(os.PathSeparator) + raw} {
			t.Run(path, func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				_, proceed, err := Parse([]string{path}, &stdout, &stderr)
				if proceed || err == nil {
					t.Fatalf("принят путь со ссылкой: proceed=%t err=%v", proceed, err)
				}
				if !strings.Contains(stderr.String(), "символической ссылкой") || stdout.Len() != 0 {
					t.Fatalf("неверная диагностика: stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
			})
		}
	}
}

func TestParseAcceptsEmptyPathAsCurrentDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	want, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{""}, {"--", ""}, {"--replace", "--depth=0", ""}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			params, proceed, err := Parse(args, &stdout, &stderr)
			if err != nil || !proceed || params.Path != want {
				t.Fatalf("Parse(%q) = (%+v, %t, %v), требуется текущий каталог %q", args, params, proceed, err, want)
			}
			if args[0] == "--replace" && (!params.Replace || params.Depth == nil || *params.Depth != 0) {
				t.Fatalf("потеряны флаги: %+v", params)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("лишний вывод: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
	var stderr bytes.Buffer
	if _, proceed, err := Parse(nil, &bytes.Buffer{}, &stderr); proceed || err == nil {
		t.Fatal("отсутствующий аргумент нельзя считать пустой строкой")
	}
}

func TestParsePreservesOrdinaryPaths(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.Mkdir("nested", 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"source.TxT", "текст с пробелами.txt", "--source.txt", " "} {
		if name == " " {
			if err := os.Mkdir(name, 0o700); err != nil {
				t.Fatal(err)
			}
			continue
		}
		writeTestFile(t, ".", name)
	}
	for _, path := range []string{".", "nested/..", "nested/../source.TxT", "./source.TxT", "текст с пробелами.txt", "--source.txt", " ", "nested/"} {
		t.Run(path, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			params, proceed, err := Parse([]string{"--", path}, &stdout, &stderr)
			want, absErr := filepath.Abs(path)
			if absErr != nil {
				t.Fatal(absErr)
			}
			if err != nil || !proceed || params.Path != want || params.Depth != nil || params.Replace {
				t.Fatalf("Parse(%q) = (%+v, %t, %v), требуется %q", path, params, proceed, err, want)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("лишний вывод: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}
