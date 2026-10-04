package storage

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteTemporaryPreparesCompleteFile(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "empty"},
		{name: "JSON", payload: []byte("{\"text\":\"Привет <>&\"}\n")},
		{name: "large payload", payload: bytes.Repeat([]byte("0123456789abcdef"), 65536)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			target := filepath.Join(directory, "lesson.json")
			writeStorageTestFile(t, target, []byte("original"))
			first, err := writeTemporary(target, test.payload)
			if err != nil {
				t.Fatalf("подготовка файла: %v", err)
			}
			second, err := writeTemporary(target, test.payload)
			if err != nil {
				t.Fatalf("подготовка второго файла: %v", err)
			}
			if first == second || first == target || second == target {
				t.Fatalf("имена временных файлов не уникальны: %q, %q", first, second)
			}
			for _, path := range []string{first, second} {
				if filepath.Dir(path) != directory {
					t.Fatalf("временный файл %q создан вне %q", path, directory)
				}
				assertStorageTestContent(t, path, test.payload)
				if err := os.Remove(path); err != nil {
					t.Fatalf("удаление подготовленного файла: %v", err)
				}
			}
			assertStorageTestContent(t, target, []byte("original"))
			assertStorageTestEntries(t, directory, "lesson.json")
		})
	}
}

func TestWriteTemporaryRejectsInvalidDirectory(t *testing.T) {
	for _, name := range []string{"missing directory", "file component"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			component := filepath.Join(directory, "parent")
			if name == "file component" {
				writeStorageTestFile(t, component, []byte("unchanged"))
			}
			path, err := writeTemporary(filepath.Join(component, "lesson.json"), []byte("new"))
			if err == nil || path != "" {
				t.Fatalf("получено (%q, %v), требуется пустой путь и ошибка", path, err)
			}
			var pathErr *os.PathError
			if !errors.As(err, &pathErr) {
				t.Errorf("не сохранена файловая ошибка: %v", err)
			}
			if name == "file component" {
				assertStorageTestContent(t, component, []byte("unchanged"))
				assertStorageTestEntries(t, directory, "parent")
			} else {
				assertStorageTestEntries(t, directory)
			}
		})
	}
}

func TestInstallNewCreatesHardLinkAndRetainsTemporaryName(t *testing.T) {
	directory := t.TempDir()
	temporary := filepath.Join(directory, "prepared")
	target := filepath.Join(directory, "lesson.json")
	payload := []byte("{\"new\":true}\n")
	writeStorageTestFile(t, temporary, payload)
	if err := installNew(temporary, target); err != nil {
		t.Fatalf("установка: %v", err)
	}
	preparedInfo, err := os.Stat(temporary)
	if err != nil {
		t.Fatal(err)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(preparedInfo, targetInfo) {
		t.Fatal("целевое и временное имена не указывают на один файл")
	}
	assertStorageTestContent(t, temporary, payload)
	if err := os.Remove(temporary); err != nil {
		t.Fatal(err)
	}
	assertStorageTestContent(t, target, payload)
}

func TestInstallNewPreservesOccupiedTarget(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			temporary := filepath.Join(directory, "prepared")
			target := filepath.Join(directory, "lesson.json")
			referent := filepath.Join(directory, "referent")
			writeStorageTestFile(t, temporary, []byte("new"))
			switch kind {
			case "file":
				writeStorageTestFile(t, target, []byte("original"))
			case "directory":
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				writeStorageTestFile(t, filepath.Join(target, "child"), []byte("original"))
			case "symlink", "dangling symlink":
				if kind == "symlink" {
					writeStorageTestFile(t, referent, []byte("original"))
				}
				if err := os.Symlink(referent, target); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Lstat(target)
			if err != nil {
				t.Fatal(err)
			}
			if err := installNew(temporary, target); !errors.Is(err, os.ErrExist) {
				t.Fatalf("требуется ошибка занятого имени, получено: %v", err)
			}
			after, err := os.Lstat(target)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatalf("существующий целевой объект изменён: %v", err)
			}
			assertStorageTestContent(t, temporary, []byte("new"))
			switch kind {
			case "file":
				assertStorageTestContent(t, target, []byte("original"))
			case "directory":
				assertStorageTestContent(t, filepath.Join(target, "child"), []byte("original"))
			case "symlink", "dangling symlink":
				if link, err := os.Readlink(target); err != nil || link != referent {
					t.Fatalf("ссылка изменена: %q, %v", link, err)
				}
				if kind == "symlink" {
					assertStorageTestContent(t, referent, []byte("original"))
				} else if _, err := os.Lstat(referent); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("объект по отсутствующему адресу ссылки появился: %v", err)
				}
			}
		})
	}
}

func TestReplaceExistingInstallsOrRestoresOriginal(t *testing.T) {
	for _, prepared := range []bool{true, false} {
		name := "restore after failed installation"
		if prepared {
			name = "replace successfully"
		}
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			temporary := filepath.Join(directory, "prepared")
			target := filepath.Join(directory, "lesson.json")
			writeStorageTestFile(t, target, []byte("original"))
			originalInfo, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			if prepared {
				writeStorageTestFile(t, temporary, []byte("new"))
			}
			replaced, err := replaceExisting(temporary, target)
			if replaced != prepared || (prepared && err != nil) || (!prepared && !errors.Is(err, os.ErrNotExist)) {
				t.Fatalf("replaceExisting = (%t, %v), наличие подготовленного файла: %t", replaced, err, prepared)
			}
			if prepared {
				assertStorageTestContent(t, target, []byte("new"))
				assertStorageTestContent(t, temporary, []byte("new"))
				assertStorageTestEntries(t, directory, "lesson.json", "prepared")
			} else {
				assertStorageTestContent(t, target, []byte("original"))
				restoredInfo, err := os.Stat(target)
				if err != nil || !os.SameFile(originalInfo, restoredInfo) {
					t.Fatalf("исходный файл не восстановлен: %v", err)
				}
				assertStorageTestEntries(t, directory, "lesson.json")
			}
		})
	}
}

func TestReplaceExistingRemovesReservationWhenOriginalIsMissing(t *testing.T) {
	directory := t.TempDir()
	temporary := filepath.Join(directory, "prepared")
	writeStorageTestFile(t, temporary, []byte("new"))
	replaced, err := replaceExisting(temporary, filepath.Join(directory, "missing.json"))
	if replaced || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("требуется отказ резервирования исходного файла, получено (%t, %v)", replaced, err)
	}
	assertStorageTestContent(t, temporary, []byte("new"))
	assertStorageTestEntries(t, directory, "prepared")
}

func writeStorageTestFile(t *testing.T, path string, payload []byte) {
	t.Helper()
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
}

func assertStorageTestContent(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("содержимое %q отличается от ожидаемых %d байт (получено %d)", path, len(want), len(got))
	}
}

func assertStorageTestEntries(t *testing.T, directory string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(want) {
		t.Fatalf("содержимое каталога: %v, требуется %v", entries, want)
	}
	for i, entry := range entries {
		if entry.Name() != want[i] {
			t.Errorf("имя объекта = %q, требуется %q", entry.Name(), want[i])
		}
	}
}
