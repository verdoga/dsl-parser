// Package discovery находит обычные TXT-файлы по параметрам console.Params
// и возвращает их абсолютные пути для последующей обработки. Пакет не
// открывает содержимое файлов, не создаёт io.Reader, не ищет JSON, не
// сопоставляет идентификаторы и не следует по символическим ссылкам.
package discovery

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/verdoga/dsl-parser/console"
)

// Find определяет тип params.Path по файловой системе. Для обычного TXT-файла
// возвращает срез с этим путём, для каталога — обычные TXT-файлы в пределах
// params.Depth; nil означает обход без ограничения, 0 — только сам каталог.
// Расширение сопоставляется без учёта регистра. Результат отсортирован
// лексикографически по абсолютным очищенным путям, в том числе при ошибке.
// Явную ссылку и неподходящий файл отклоняет. При ошибке чтения подкаталога
// возвращает также уже найденные пути и ошибку; не печатает сообщения.
// Если подходящих файлов нет и обход прошёл без ошибок, возвращает пустой
// срез и nil. Поле params.Replace не влияет на поиск.
func Find(params console.Params) (paths []string, err error) {
	path := params.Path
	if path == "" {
		path = "."
	}
	var info os.FileInfo
	// Сначала проверяем исходную запись, затем абсолютную форму с её родителями.
	for pass := 0; pass < 2; pass++ {
		volumeLen := len(filepath.VolumeName(path))
		for i := volumeLen; i <= len(path); i++ {
			if i < len(path) && (i <= volumeLen || !os.IsPathSeparator(path[i])) {
				continue
			}
			component := path[:i]
			info, err = os.Lstat(component)
			if err != nil {
				return nil, fmt.Errorf("не удалось проверить исходный путь %q: %w", component, err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("компонент исходного пути %q является символической ссылкой", component)
			}
		}
		if pass == 0 {
			path, err = filepath.Abs(path)
			if err != nil {
				return nil, fmt.Errorf("не удалось получить абсолютный путь для %q: %w", params.Path, err)
			}
		}
	}
	if info.IsDir() {
		paths, err = walkDirectory(path, params.Depth)
		slices.Sort(paths)
		return paths, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("исходный путь %q не является обычным файлом или каталогом", path)
	}
	if !isTXT(path) {
		return nil, fmt.Errorf("исходный файл %q должен иметь расширение .txt", path)
	}
	return []string{path}, nil
}

// walkDirectory собирает пути обычных TXT-файлов от корневого каталога
// до maxDepth уровней подкаталогов включительно. Ссылки на файлы и каталоги
// пропускает, ошибки доступа накапливает без прекращения обхода доступных
// подкаталогов и возвращает вместе с найденными путями.
func walkDirectory(root string, maxDepth *int) (paths []string, err error) {
	paths = []string{}
	info, statErr := os.Lstat(root)
	if statErr != nil {
		return paths, fmt.Errorf("не удалось проверить каталог %q: %w", root, statErr)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return paths, nil
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		err = fmt.Errorf("не удалось прочитать каталог %q: %w", root, readErr)
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, statErr := entry.Info()
		if statErr != nil {
			err = errors.Join(err, fmt.Errorf("не удалось проверить путь %q: %w", path, statErr))
			continue
		}
		if info.IsDir() {
			if maxDepth != nil && *maxDepth <= 0 {
				continue
			}
			var nextDepth *int
			if maxDepth != nil {
				depth := *maxDepth - 1
				nextDepth = &depth
			}
			found, walkErr := walkDirectory(path, nextDepth)
			paths = append(paths, found...)
			err = errors.Join(err, walkErr)
			continue
		}
		if info.Mode().IsRegular() && isTXT(path) {
			paths = append(paths, path)
		}
	}
	return paths, err
}

// isTXT сообщает, оканчивается ли имя пути расширением .txt без учёта
// регистра; проверка типа файла выполняется отдельно по файловой системе.
func isTXT(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".txt")
}
