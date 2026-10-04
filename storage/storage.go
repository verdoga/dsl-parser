package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/verdoga/dsl-parser/console"
	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
)

// Save сохраняет готовый result одного TXT-файла и возвращает только Report.
// Путь отдельного файла берёт из result.Document.FilePath: params.Path может
// указывать на каталог. Сначала проверяет существование целевого JSON и
// params.Replace, затем диагностики с Fatal=true. При запрете замены или
// фатальной диагностике не создаёт и не заменяет JSON, возвращая причину в
// Report.Err. Нефатальные диагностики не препятствуют записи.
// Ошибка сериализации или записи также помещается в Report.Err. Action
// отражает факт установки нового JSON, включая последующую ошибку очистки.
// Save не изменяет result и не печатает сообщения.
func Save(result *model.Result, params console.Params) Report {
	errorCount := 0
	if result != nil {
		for _, diagnostic := range result.Diagnostics {
			if diagnostic.SeverityLevel == diagnostics.SeverityError {
				errorCount++
			}
		}
	}
	sourcePath, targetPath, err := outputPath(result)
	if err != nil {
		return NewReport(sourcePath, targetPath, ActionNotWritten, errorCount, err)
	}
	exists, err := checkTarget(targetPath, params.Replace)
	if err != nil {
		return NewReport(sourcePath, targetPath, ActionNotWritten, errorCount, err)
	}
	var fatalErrors []error
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Fatal {
			fatalErrors = append(fatalErrors, fmt.Errorf("%s: %s", diagnostic.DiagnosticCode, diagnostic.Message))
		}
	}
	if err := errors.Join(fatalErrors...); err != nil {
		return NewReport(sourcePath, targetPath, ActionNotWritten, errorCount, err)
	}
	payload, err := encode(result)
	if err != nil {
		return NewReport(sourcePath, targetPath, ActionNotWritten, errorCount, err)
	}
	temporaryPath, err := writeTemporary(targetPath, payload)
	if err != nil {
		return NewReport(sourcePath, targetPath, ActionNotWritten, errorCount, err)
	}
	action := ActionNotWritten
	if exists {
		var replaced bool
		replaced, err = replaceExisting(temporaryPath, targetPath)
		if replaced {
			action = ActionReplaced
		}
	} else {
		err = installNew(temporaryPath, targetPath)
		if err == nil {
			action = ActionCreated
		}
	}
	if removeErr := os.Remove(temporaryPath); removeErr != nil {
		err = errors.Join(err, fmt.Errorf("не удалось удалить временный файл %q: %w", temporaryPath, removeErr))
	}
	return NewReport(sourcePath, targetPath, action, errorCount, err)
}

// outputPath проверяет абсолютный очищенный Document.FilePath и его
// согласованность с Document.FileName. Целевой путь строит заменой
// последнего расширения .txt на .json в каталоге исходного файла.
// Расширение .txt проверяется без учёта регистра.
func outputPath(result *model.Result) (sourcePath, targetPath string, err error) {
	if result == nil {
		return "", "", fmt.Errorf("модель результата отсутствует")
	}
	if result.Document.FilePath == nil || *result.Document.FilePath == "" {
		return "", "", fmt.Errorf("путь исходного TXT неизвестен или пуст")
	}
	path := *result.Document.FilePath
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", "", fmt.Errorf("путь исходного TXT %q должен быть абсолютным и очищенным", path)
	}
	if result.Document.FileName == nil || *result.Document.FileName != filepath.Base(path) {
		return path, "", fmt.Errorf("имя исходного TXT не согласовано с путём %q", path)
	}
	extension := filepath.Ext(path)
	if !strings.EqualFold(extension, ".txt") {
		return path, "", fmt.Errorf("исходный файл %q должен иметь расширение .txt", path)
	}
	return path, strings.TrimSuffix(path, extension) + ".json", nil
}

// checkTarget проверяет целевой путь до проверки фатальных диагностик.
// exists=true означает существующий обычный JSON. При exists и !replace
// возвращает ошибку запрета замены. Каталог, ссылка и ошибка доступа
// также возвращаются как err без изменения существующего файла.
func checkTarget(targetPath string, replace bool) (exists bool, err error) {
	info, err := os.Lstat(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("не удалось проверить целевой JSON %q: %w", targetPath, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("целевой путь %q не является обычным файлом", targetPath)
	}
	if !replace {
		return true, fmt.Errorf("замена целевого JSON %q запрещена: %w", targetPath, os.ErrExist)
	}
	return true, nil
}

// encode сериализует проверенный Result в JSON контракта 1.0 с отступом
// два пробела, завершающим LF и без HTML-экранирования <, > и &.
// Ошибка возвращается до создания временного файла.
// Проверяет наличие модели и FormatVersion1; остальные инварианты
// обеспечивает вызывающий код. Значения модели не исправляются.
func encode(result *model.Result) (payload []byte, err error) {
	if result == nil {
		return nil, fmt.Errorf("модель результата отсутствует")
	}
	if result.Format != model.FormatVersion1 {
		return nil, fmt.Errorf("неподдерживаемый формат модели %q", result.Format)
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		return nil, fmt.Errorf("не удалось сериализовать модель результата: %w", err)
	}
	return buffer.Bytes(), nil
}
