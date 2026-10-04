package discovery

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/verdoga/dsl-parser/console"
)

func writeTXTFixture(t *testing.T, root, name string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("произвольное содержимое"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestIsTXTChecksOnlyExtension(t *testing.T) {
	for _, test := range []struct {
		path string
		want bool
	}{
		{"a.txt", true}, {"a.TxT", true}, {".txt", true},
		{"несуществующий/текст.TXT", true}, {"", false}, {"txt", false},
		{"a.txt.bak", false}, {"a.txt/child", false}, {"a.txt/", false},
	} {
		t.Run(test.path, func(t *testing.T) {
			if got := isTXT(test.path); got != test.want {
				t.Fatalf("isTXT(%q) = %t, требуется %t", test.path, got, test.want)
			}
		})
	}
}

func TestFindSingleFilePreservesParams(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	file := writeTXTFixture(t, root, "nested/текст.TxT")
	if err := os.Chmod(file, 0); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, "nested/../nested/текст.TxT"} {
		zero, two := 0, 2
		for _, depth := range []*int{nil, &zero, &two} {
			wantDepth := 0
			if depth != nil {
				wantDepth = *depth
			}
			for _, replace := range []bool{false, true} {
				params := console.Params{Path: path, Depth: depth, Replace: replace}
				got, err := Find(params)
				if err != nil || !slices.Equal(got, []string{file}) {
					t.Fatalf("Find(%+v) = %v, %v", params, got, err)
				}
				if params.Path != path || params.Depth != depth || params.Replace != replace || depth != nil && *depth != wantDepth {
					t.Fatalf("изменены параметры: %+v", params)
				}
			}
		}
	}
}

func TestFindRejectsInvalidSource(t *testing.T) {
	root := t.TempDir()
	wrong := writeTXTFixture(t, root, "source.json")
	for _, path := range []string{wrong, filepath.Join(root, "missing.txt"), os.DevNull} {
		t.Run(path, func(t *testing.T) {
			got, err := Find(console.Params{Path: path})
			if err == nil || len(got) != 0 {
				t.Fatalf("Find(%q) = %v, %v, требуется ошибка без путей", path, got, err)
			}
			if filepath.Base(path) == "missing.txt" && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("потеряна причина отсутствия файла: %v", err)
			}
		})
	}
}

func TestFindDepthAndGlobalSorting(t *testing.T) {
	root := t.TempDir()
	wantByDepth := [][]string{
		{writeTXTFixture(t, root, "a.txt"), writeTXTFixture(t, root, "z.TXT")},
		{writeTXTFixture(t, root, "a/child.txt"), writeTXTFixture(t, root, "folder.txt/child.txt")},
		{writeTXTFixture(t, root, "a/b/grandchild.txt")},
	}
	writeTXTFixture(t, root, "ignored.json")
	for _, limit := range []int{0, 1, 2, 3, -1} {
		var depth *int
		depthValue := limit
		if limit >= 0 {
			depth = &depthValue
		}
		var want []string
		for level, paths := range wantByDepth {
			if depth == nil || level <= *depth {
				want = append(want, paths...)
			}
		}
		slices.Sort(want)
		for _, replace := range []bool{false, true} {
			got, err := Find(console.Params{Path: root, Depth: depth, Replace: replace})
			if err != nil || !slices.Equal(got, want) || depth != nil && *depth != limit {
				t.Fatalf("глубина %d, replace=%t: %v, %v; требуется %v", limit, replace, got, err, want)
			}
		}
		got, err := walkDirectory(root, depth)
		slices.Sort(got)
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("walkDirectory, глубина %d: %v, %v; требуется %v", limit, got, err, want)
		}
	}
}

func TestFindEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	writeTXTFixture(t, root, "ignored.txt.bak")
	got, err := Find(console.Params{Path: root})
	if err != nil || len(got) != 0 {
		t.Fatalf("требуется пустой успешный результат: %v, %v", got, err)
	}
}

func TestFindRejectsSourceSymlinksAndSkipsWalkSymlinks(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	file := writeTXTFixture(t, root, "target/source.txt")
	external := t.TempDir()
	writeTXTFixture(t, external, "outside.txt")
	for name, target := range map[string]string{
		"link": "target", "file.txt": "target/source.txt", "dangling.txt": "missing",
		"target/cycle": ".", "target/external": external,
	} {
		if err := os.Symlink(target, name); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{
		"link", "link/source.txt", "link/../target/source.txt", "link/", "link/.",
		"file.txt", "file.txt/../target/source.txt", "dangling.txt", "dangling.txt/../target/source.txt",
	} {
		for _, path := range []string{raw, root + string(os.PathSeparator) + raw} {
			t.Run(path, func(t *testing.T) {
				got, err := Find(console.Params{Path: path})
				if err == nil || len(got) != 0 {
					t.Fatalf("принята ссылка %q: %v, %v", path, got, err)
				}
			})
		}
	}
	got, err := Find(console.Params{Path: root})
	if err != nil || !slices.Equal(got, []string{file}) {
		t.Fatalf("обход ссылок: %v, %v; требуется только %q", got, err, file)
	}
}

func TestFindRechecksParentAfterConsoleParse(t *testing.T) {
	root := t.TempDir()
	file := writeTXTFixture(t, root, "parent/source.txt")
	params, proceed, err := console.Parse([]string{file}, io.Discard, io.Discard)
	if err != nil || !proceed {
		t.Fatalf("console.Parse: proceed=%t, err=%v", proceed, err)
	}
	parent, moved := filepath.Join(root, "parent"), filepath.Join(root, "moved")
	if err := os.Rename(parent, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(moved, parent); err != nil {
		t.Fatal(err)
	}
	if got, err := Find(params); err == nil || len(got) != 0 {
		t.Fatalf("принят подменённый родитель: %v, %v", got, err)
	}
}

func TestFindAccumulatesAccessErrorsAndContinues(t *testing.T) {
	root := t.TempDir()
	want := []string{writeTXTFixture(t, root, "b.txt"), writeTXTFixture(t, root, "b/inside.txt"), writeTXTFixture(t, root, "z/last.txt")}
	denied := []string{filepath.Join(root, "a-denied"), filepath.Join(root, "c-denied")}
	for _, path := range denied {
		writeTXTFixture(t, path, "hidden.txt")
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(path, 0o700); err != nil {
				t.Error(err)
			}
		})
		if _, err := os.ReadDir(path); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("среда не обеспечивает запрет чтения %q: %v", path, err)
		}
		got, err := Find(console.Params{Path: path})
		if len(got) != 0 || !errors.Is(err, os.ErrPermission) {
			t.Fatalf("недоступный корень: %v, %v", got, err)
		}
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) || pathErr.Path != path {
			t.Fatalf("потерян контекст ошибки пути: %v", err)
		}
	}
	got, err := Find(console.Params{Path: root})
	if !slices.Equal(got, want) || !errors.Is(err, os.ErrPermission) {
		t.Fatalf("частичный результат: %v, %v; требуется %v и ошибка доступа", got, err, want)
	}
	var failedPaths []string
	pending := []error{err}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		switch current := current.(type) {
		case *os.PathError:
			if !errors.Is(current, os.ErrPermission) {
				t.Fatalf("неожиданная причина: %v", current)
			}
			failedPaths = append(failedPaths, current.Path)
		case interface{ Unwrap() []error }:
			pending = append(pending, current.Unwrap()...)
		case interface{ Unwrap() error }:
			pending = append(pending, current.Unwrap())
		}
	}
	slices.Sort(failedPaths)
	if !slices.Equal(failedPaths, denied) {
		t.Fatalf("потеряны ошибки каталогов: %v, требуется %v", failedPaths, denied)
	}
	zero := 0
	got, err = Find(console.Params{Path: root, Depth: &zero})
	if err != nil || !slices.Equal(got, want[:1]) {
		t.Fatalf("прочитаны каталоги за пределом глубины: %v, %v", got, err)
	}
}

func TestFindDoesNotPrint(t *testing.T) {
	root := t.TempDir()
	file := writeTXTFixture(t, root, "source.txt")
	output, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { output.Close() })
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = output, output
	t.Cleanup(func() { os.Stdout, os.Stderr = stdout, stderr })
	for _, path := range []string{root, file, filepath.Join(root, "missing")} {
		Find(console.Params{Path: path})
	}
	info, err := output.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Fatalf("Find напечатала %d байт", info.Size())
	}
}
