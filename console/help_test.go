package console

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPrintHelp(t *testing.T) {
	const want = `Использование: dslparser [--replace] [--depth N] <path>

Аргументы:
  <path>       путь к обычному TXT-файлу или каталогу

Флаги:
  --replace    разрешить повторный разбор и замену целевого JSON
  --depth N    ограничить максимальное число уровней подкаталогов
  --help, -h   показать эту справку

Если --depth не указан, глубина не ограничена. Значение 0 ограничивает поиск
переданным каталогом. Положительное N задаёт максимальное число уровней
подкаталогов. Для пути к отдельному файлу значение --depth игнорируется.
`
	var output bytes.Buffer

	err := PrintHelp(&output)
	if err != nil {
		t.Fatalf("PrintHelp() вернул ошибку: %v", err)
	}
	if output.String() != want {
		t.Errorf("PrintHelp() вывел:\n%q\nтребуется:\n%q", output.String(), want)
	}
}

func TestPrintHelpDescribesCommandOptions(t *testing.T) {
	var output bytes.Buffer
	if err := PrintHelp(&output); err != nil {
		t.Fatalf("PrintHelp() вернул ошибку: %v", err)
	}

	parts := []string{
		"dslparser [--replace] [--depth N] <path>",
		"путь к обычному TXT-файлу или каталогу",
		"--replace",
		"--depth N",
		"--help, -h",
		"глубина не ограничена",
		"Значение 0 ограничивает поиск",
		"Положительное N задаёт максимальное число уровней",
		"Для пути к отдельному файлу значение --depth игнорируется",
	}
	for _, part := range parts {
		t.Run(part, func(t *testing.T) {
			if !strings.Contains(output.String(), part) {
				t.Errorf("справка не содержит %q", part)
			}
		})
	}
}

func TestPrintHelpReturnsWriteError(t *testing.T) {
	writeErr := errors.New("ошибка записи")

	err := PrintHelp(errorWriter{err: writeErr})

	if !errors.Is(err, writeErr) {
		t.Errorf("PrintHelp() error = %v, требуется %v", err, writeErr)
	}
}

type errorWriter struct {
	err error
}

func (writer errorWriter) Write([]byte) (int, error) {
	return 0, writer.err
}
