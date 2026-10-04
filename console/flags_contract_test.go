package console

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseFlagErrorsAreRussian(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--unknown"}, `неизвестный флаг "unknown"`},
		{[]string{"-unknown=x"}, `неизвестный флаг "unknown"`},
		{[]string{"--help", "--unknown"}, `неизвестный флаг "unknown"`},
		{[]string{"--depth"}, "для флага --depth требуется значение"},
		{[]string{"-depth"}, "для флага --depth требуется значение"},
		{[]string{"--depth="}, `глубина "" не является целым числом`},
		{[]string{"--depth=bad"}, `глубина "bad" не является целым числом`},
		{[]string{"--depth", "--help"}, `глубина "--help" не является целым числом`},
		{[]string{"--depth=-1"}, "глубина не может быть отрицательной: -1"},
		{[]string{"--depth=999999999999999999999999"}, `глубина "999999999999999999999999" выходит за допустимый диапазон целых чисел`},
		{[]string{"--replace=bad"}, `недопустимое значение "bad" флага --replace: ожидается логическое значение, например true или false`},
		{[]string{"--help="}, `недопустимое значение "" флага --help: ожидается логическое значение, например true или false`},
		{[]string{"-h=bad"}, `недопустимое значение "bad" флага --h: ожидается логическое значение, например true или false`},
		{[]string{"---help"}, `некорректная запись флага "---help"`},
		{[]string{"--=x"}, `некорректная запись флага "--=x"`},
	}
	for _, test := range tests {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			_, proceed, err := Parse(test.args, &stdout, &stderr)
			if proceed || err == nil {
				t.Fatalf("ошибка не обнаружена: proceed=%t err=%v", proceed, err)
			}
			want := "Ошибка: не удалось разобрать параметры: " + test.want + "\n"
			if stderr.String() != want || stdout.Len() != 0 {
				t.Fatalf("stdout=%q stderr=%q, требуется stderr=%q", stdout.String(), stderr.String(), want)
			}
		})
	}
}

func TestParseFlagsPreservesSupportedSyntax(t *testing.T) {
	for _, test := range []struct {
		args    []string
		path    string
		replace bool
		help    bool
	}{
		{args: []string{"--replace=false", "source"}, path: "source"},
		{args: []string{"--replace=1", "source"}, path: "source", replace: true},
		{args: []string{"--replace", "--replace=0", "source"}, path: "source"},
		{args: []string{"--help=false", "source"}, path: "source"},
		{args: []string{"-h=0", "source"}, path: "source"},
		{args: []string{"--help=true"}, help: true},
		{args: []string{"-h=1"}, help: true},
		{args: []string{"--replace", "false"}, path: "false", replace: true},
		{args: []string{"--", "--help"}, path: "--help"},
		{args: []string{"-"}, path: "-"},
		{args: []string{"--help", "one", "two"}, help: true},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			params, help, err := parseFlags(test.args)
			if err != nil || help != test.help || params.Path != test.path || params.Replace != test.replace || params.Depth != nil {
				t.Fatalf("parseFlags(%q) = (%+v, %t, %v)", test.args, params, help, err)
			}
		})
	}
	first, _, err := parseFlags([]string{"--depth=0", "--depth", "+3", "source"})
	if err != nil || first.Depth == nil || *first.Depth != 3 {
		t.Fatalf("повторный --depth не переопределил значение: %+v, %v", first, err)
	}
	second, _, err := parseFlags([]string{"source"})
	if err != nil || second.Depth != nil || *first.Depth != 3 {
		t.Fatalf("состояние разборов связано: first=%+v second=%+v err=%v", first, second, err)
	}
}
