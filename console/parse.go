package console

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Parse разбирает args без имени программы, проверяет путь и выводит справку или
// ошибку параметров на соответствующий поток. proceed=true означает, что Params
// действительны и app может продолжить обработку; proceed=false, err=nil означает
// запрос справки, а proceed=false, err!=nil — ошибку запуска либо вывода.
// Ошибка уже напечатана здесь, поэтому app использует err только для выбора кода
// завершения: 0 для справки, 2 для ошибки, и повторно её не печатает.
func Parse(args []string, stdout, stderr io.Writer) (params Params, proceed bool, err error) {
	params, help, err := parseFlags(args)
	if err == nil && help {
		if err := PrintHelp(stdout); err != nil {
			return params, false, fmt.Errorf("не удалось вывести справку: %w", err)
		}
		return params, false, nil
	}
	if err == nil {
		params.Path, err = normalizePath(params.Path)
	}
	if err == nil {
		err = validateSourcePath(params.Path)
	}
	if err != nil {
		if _, writeErr := fmt.Fprintf(stderr, "Ошибка: %v\n", err); writeErr != nil {
			return params, false, fmt.Errorf("не удалось вывести ошибку %q: %w", err, writeErr)
		}
		return params, false, err
	}

	return params, true, nil
}

// parseFlags регистрирует --replace, --depth и формы справки --help/-h через
// локальный набор флагов; проверяет один обязательный позиционный путь и не
// допускает неизвестных флагов. При help=true дальнейшая обработка не нужна.
func parseFlags(args []string) (params Params, help bool, err error) {
	flags := flag.NewFlagSet("dslparser", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	replace := flags.Bool("replace", false, "разрешить замену целевого JSON")
	helpLong := flags.Bool("help", false, "показать справку")
	helpShort := flags.Bool("h", false, "показать справку")
	flags.Func("depth", "максимальное число уровней подкаталогов", func(raw string) error {
		depth, err := parseDepth(raw)
		if err != nil {
			return err
		}
		params.Depth = &depth
		return nil
	})

	if err := flags.Parse(args); err != nil {
		return Params{}, false, fmt.Errorf("не удалось разобрать параметры: %w", err)
	}
	params.Replace = *replace
	if *helpLong || *helpShort {
		return params, true, nil
	}
	if flags.NArg() != 1 {
		return Params{}, false, fmt.Errorf("требуется ровно один исходный путь, получено: %d", flags.NArg())
	}
	params.Path = flags.Arg(0)

	return params, false, nil
}

// parseDepth преобразует аргумент --depth в целое число не меньше нуля.
func parseDepth(raw string) (depth int, err error) {
	depth, err = strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("глубина %q не является целым числом: %w", raw, err)
	}
	if depth < 0 {
		return 0, fmt.Errorf("глубина не может быть отрицательной: %d", depth)
	}
	return depth, nil
}

// normalizePath делает указанный путь абсолютным и очищенным.
func normalizePath(raw string) (path string, err error) {
	path, err = filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("не удалось получить абсолютный путь для %q: %w", raw, err)
	}
	return path, nil
}

// validateSourcePath проверяет существование обычного файла или каталога;
// явно переданный файл должен иметь расширение .txt без учёта регистра.
// Символические ссылки в исходном пути не разрешены.
func validateSourcePath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("не удалось проверить исходный путь %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("исходный путь %q является символической ссылкой", path)
	}
	if info.IsDir() {
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("исходный путь %q не является обычным файлом или каталогом", path)
	}
	if !strings.EqualFold(filepath.Ext(path), ".txt") {
		return fmt.Errorf("исходный файл %q должен иметь расширение .txt", path)
	}
	return nil
}
