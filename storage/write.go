package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// writeTemporary создаёт временный файл в каталоге targetPath, полностью
// записывает payload, синхронизирует и закрывает его до установки JSON.
// При ошибке удаляет незавершённый временный файл и возвращает err.
func writeTemporary(targetPath string, payload []byte) (temporaryPath string, err error) {
	file, err := os.CreateTemp(filepath.Dir(targetPath), ".storage-*.tmp")
	if err != nil {
		return "", fmt.Errorf("не удалось создать временный файл для %q: %w", targetPath, err)
	}
	temporaryPath = file.Name()
	written, writeErr := file.Write(payload)
	if writeErr == nil && written != len(payload) {
		writeErr = io.ErrShortWrite
	}
	if writeErr != nil {
		err = fmt.Errorf("не удалось записать временный файл %q: %w", temporaryPath, writeErr)
	} else if syncErr := file.Sync(); syncErr != nil {
		err = fmt.Errorf("не удалось синхронизировать временный файл %q: %w", temporaryPath, syncErr)
	}
	if closeErr := file.Close(); closeErr != nil {
		err = errors.Join(err, fmt.Errorf("не удалось закрыть временный файл %q: %w", temporaryPath, closeErr))
	}
	if err != nil {
		if removeErr := os.Remove(temporaryPath); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("не удалось удалить временный файл %q: %w", temporaryPath, removeErr))
		}
		return "", err
	}
	return temporaryPath, nil
}

// installNew устанавливает подготовленный файл на свободный целевой путь.
// При ошибке не обнуляет существующий целевой файл и возвращает err.
// После успеха временное имя остаётся для очистки вызывающим кодом.
func installNew(temporaryPath, targetPath string) error {
	if err := os.Link(temporaryPath, targetPath); err != nil {
		return fmt.Errorf("не удалось установить JSON %q из %q: %w", targetPath, temporaryPath, err)
	}
	return nil
}

// replaceExisting резервно переименовывает существующий файл и устанавливает
// подготовленный. При неудачной установке пытается восстановить прежний файл;
// ошибка восстановления входит в err. replaced=true означает, что новый JSON
// установлен, даже если последующее удаление резерва завершилось ошибкой.
// При replaced=false новый JSON не установлен. Временное имя очищает вызывающий код.
func replaceExisting(temporaryPath, targetPath string) (replaced bool, err error) {
	backup, err := os.CreateTemp(filepath.Dir(targetPath), ".storage-*.bak")
	if err != nil {
		return false, fmt.Errorf("не удалось подготовить резерв для %q: %w", targetPath, err)
	}
	backupPath := backup.Name()
	if closeErr := backup.Close(); closeErr != nil {
		err = fmt.Errorf("не удалось закрыть резервный файл %q: %w", backupPath, closeErr)
		if removeErr := os.Remove(backupPath); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("не удалось удалить резервный файл %q: %w", backupPath, removeErr))
		}
		return false, err
	}
	if renameErr := os.Rename(targetPath, backupPath); renameErr != nil {
		err = fmt.Errorf("не удалось перенести JSON %q в резерв %q: %w", targetPath, backupPath, renameErr)
		if removeErr := os.Remove(backupPath); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("не удалось удалить резервный файл %q: %w", backupPath, removeErr))
		}
		return false, err
	}
	if err = installNew(temporaryPath, targetPath); err != nil {
		if restoreErr := os.Rename(backupPath, targetPath); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("не удалось восстановить JSON %q из резерва %q: %w", targetPath, backupPath, restoreErr))
		}
		return false, err
	}
	if removeErr := os.Remove(backupPath); removeErr != nil {
		return true, fmt.Errorf("не удалось удалить резервный файл %q: %w", backupPath, removeErr)
	}
	return true, nil
}
