package console

import (
	"errors"
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
// Пустой позиционный путь означает текущий каталог; отсутствие аргумента — ошибка.
// Ошибки параметров печатаются здесь, поэтому app использует err только для выбора
// кода завершения: 0 для справки, 2 для ошибки, и повторно её не печатает.
// Ошибки вывода только возвращаются: в частности, сбой печати справки намеренно
// не сопровождается диагностикой в stderr.
func Parse(args []string, stdout, stderr io.Writer) (params Params, proceed bool, err error) {
	params, help, err := parseFlags(args)
	if err == nil && help {
		if err := PrintHelp(stdout); err != nil {
			return params, false, fmt.Errorf("не удалось вывести справку: %w", err)
		}
		return params, false, nil
	}
	if err == nil {
		err = validateSourcePath(params.Path)
	}
	if err == nil {
		params.Path, err = normalizePath(params.Path)
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
// допускает неизвестных флагов. Пустой путь означает текущий каталог.
// При help=true дальнейшая обработка не нужна.
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

	paths, err := parseFlagArguments(flags, args)
	if err != nil {
		return Params{}, false, fmt.Errorf("не удалось разобрать параметры: %w", err)
	}
	params.Replace = *replace
	if *helpLong || *helpShort {
		return params, true, nil
	}
	if len(paths) != 1 {
		return Params{}, false, fmt.Errorf("требуется ровно один исходный путь, получено: %d", len(paths))
	}
	params.Path = paths[0]

	return params, false, nil
}

// parseFlagArguments задаёт зарегистрированные флаги и возвращает позиционные
// аргументы. Как flag.Parse, прекращает разбор на первом пути или после --.
// Ошибки формируются по аргументам, без разбора английских сообщений flag.
func parseFlagArguments(flags *flag.FlagSet, args []string) ([]string, error) {
	for len(args) > 0 {
		arg := args[0]
		if len(arg) < 2 || arg[0] != '-' {
			return args, nil
		}
		args = args[1:]
		if arg == "--" {
			return args, nil
		}
		name := strings.TrimPrefix(arg, "-")
		name = strings.TrimPrefix(name, "-")
		if name == "" || name[0] == '-' || name[0] == '=' {
			return nil, fmt.Errorf("некорректная запись флага %q", arg)
		}
		name, value, hasValue := strings.Cut(name, "=")
		if flags.Lookup(name) == nil {
			return nil, fmt.Errorf("неизвестный флаг %q", name)
		}
		if !hasValue {
			value = "true"
			if name == "depth" {
				if len(args) == 0 {
					return nil, fmt.Errorf("для флага --depth требуется значение")
				}
				value, args = args[0], args[1:]
			}
		}
		if err := flags.Set(name, value); err != nil {
			if name == "depth" {
				return nil, err
			}
			return nil, fmt.Errorf("недопустимое значение %q флага --%s: ожидается логическое значение, например true или false", value, name)
		}
	}
	return args, nil
}

// parseDepth преобразует аргумент --depth в целое число не меньше нуля.
func parseDepth(raw string) (depth int, err error) {
	depth, err = strconv.Atoi(raw)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, fmt.Errorf("глубина %q выходит за допустимый диапазон целых чисел", raw)
		}
		return 0, fmt.Errorf("глубина %q не является целым числом", raw)
	}
	if depth < 0 {
		return 0, fmt.Errorf("глубина не может быть отрицательной: %d", depth)
	}
	return depth, nil
}

// normalizePath делает указанный путь абсолютным и очищенным.
// Пустая строка означает текущий каталог.
func normalizePath(raw string) (path string, err error) {
	path, err = filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("не удалось получить абсолютный путь для %q: %w", raw, err)
	}
	return path, nil
}

// validateSourcePath проверяет существование обычного файла или каталога;
// явно переданный файл должен иметь расширение .txt без учёта регистра.
// Пустой путь означает текущий каталог. Символические ссылки запрещены во всех
// компонентах, в том числе перед ..; путь проверяется до очистки.
func validateSourcePath(path string) error {
	if path == "" {
		path = "."
	}
	if _, err := sourcePathInfo(path); err != nil {
		return err
	}
	path, err := normalizePath(path)
	if err != nil {
		return err
	}
	// Абсолютная форма также проверяет родителей текущего каталога,
	// отсутствующих в относительном аргументе.
	info, err := sourcePathInfo(path)
	if err != nil {
		return err
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

// sourcePathInfo проверяет компоненты пути по порядку, не удаляя . и ..,
// и возвращает сведения о последнем компоненте.
func sourcePathInfo(path string) (os.FileInfo, error) {
	volumeLen := len(filepath.VolumeName(path))
	for i := volumeLen; i < len(path); i++ {
		if i > volumeLen && os.IsPathSeparator(path[i]) {
			if _, err := sourceComponentInfo(path[:i]); err != nil {
				return nil, err
			}
		}
	}
	return sourceComponentInfo(path)
}

// sourceComponentInfo получает сведения о компоненте, запрещая ссылку на его месте.
func sourceComponentInfo(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("не удалось проверить исходный путь %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("компонент исходного пути %q является символической ссылкой", path)
	}
	return info, nil
}
