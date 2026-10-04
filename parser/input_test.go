package parser

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/gr12"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestPrepareInputFactsVersionAndEarlyDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		source, version string
		codes           []diagnostics.Code
	}{
		{"@dsl-version 1.2", "1.2", nil}, {"\uFEFF \t@DsL-Version   1.2 \t\r\nя\n", "1.2", nil},
		{"@dsl-version 1.2\n@dsl-version 2.0", "1.2", nil},
		{"@dsl-version 2.0", "2.0", []diagnostics.Code{diagnostics.P014}},
		{"@dsl-version Future", "Future", []diagnostics.Code{diagnostics.P014}},
		{"\xff\r", "", []diagnostics.Code{diagnostics.P001, diagnostics.P002}},
		{"\xff\r\n", "", []diagnostics.Code{diagnostics.P001}},
		{"@dsl-version 1.2\r", "", []diagnostics.Code{diagnostics.P002}},
		{"", "", []diagnostics.Code{diagnostics.P013}}, {"\uFEFF", "", []diagnostics.Code{diagnostics.P013}},
		{"\uFEFF\uFEFF@dsl-version 1.2", "", []diagnostics.Code{diagnostics.P013}},
		{"\n@dsl-version 1.2", "", []diagnostics.Code{diagnostics.P013}},
		{"@dsl-version", "", []diagnostics.Code{diagnostics.P013}},
		{"@dsl-version\t1.2", "", []diagnostics.Code{diagnostics.P013}},
		{"@dsl-version \t1.2", "", []diagnostics.Code{diagnostics.P013}},
		{"@dsl-version 1.2 extra", "", []diagnostics.Code{diagnostics.P013}},
		{"@dsl-version 1.\u00a02", "", []diagnostics.Code{diagnostics.P013}},
		{"@dsl-version-extra 1.2", "", []diagnostics.Code{diagnostics.P013}},
	} {
		t.Run(fmt.Sprintf("%q", tc.source), func(t *testing.T) {
			p := inputParser(t, tc.source)
			lines, ready, err := p.prepareInput()
			if err != nil || ready != (len(tc.codes) == 0) {
				t.Fatalf("prepareInput: ready=%t, err=%v", ready, err)
			}
			doc := p.result.Document
			if doc.ByteLength == nil || *doc.ByteLength != int64(len(tc.source)) || doc.SHA256 == nil || *doc.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(tc.source))) || doc.HasBOM == nil || *doc.HasBOM != strings.HasPrefix(tc.source, "\uFEFF") {
				t.Fatal("Байтовые факты не соответствуют точному источнику")
			}
			if (tc.version == "" && doc.DSLVersion != nil) || (tc.version != "" && (doc.DSLVersion == nil || *doc.DSLVersion != tc.version || p.version != tc.version)) {
				t.Fatal("Версия потеряна или подменена")
			}
			var codes []diagnostics.Code
			for _, diagnostic := range p.result.Diagnostics {
				codes = append(codes, diagnostic.DiagnosticCode)
				if !diagnostic.Fatal || diagnostic.Source != "current" || diagnostic.Location != nil {
					t.Fatal("Неверная ранняя диагностика")
				}
			}
			if !slices.Equal(codes, tc.codes) {
				t.Fatalf("Коды: %v, требуется %v", codes, tc.codes)
			}
			if p.result.Lines == nil || len(p.result.Lines) != 0 {
				t.Fatal("Подготовка сохранила неразобранные строки")
			}
			if !ready {
				if len(lines) != 0 || doc.LineCount != nil {
					t.Fatal("Ранний отказ сохранил строки или LineCount")
				}
				if err := validateResult(p.result); err != nil {
					t.Fatal(err)
				}
				return
			}
			if doc.Encoding == nil || *doc.Encoding != "UTF-8" || doc.LineCount == nil || *doc.LineCount != len(lines) || p.seenMetadata["@dsl-version"] != 1 {
				t.Fatal("Подготовка не завершена")
			}
			var restored strings.Builder
			for _, line := range lines {
				restored.WriteString(line.Raw)
				restored.WriteString(string(line.LineEnding))
			}
			if restored.String() != strings.TrimPrefix(tc.source, "\uFEFF") {
				t.Fatal("Исходные строки нормализованы")
			}
		})
	}
}

func TestSplitLinesPreservesPhysicalLines(t *testing.T) {
	for _, tc := range []struct {
		source   string
		raw, eol []string
	}{
		{"", nil, nil}, {"\uFEFF", nil, nil}, {"x", []string{"x"}, []string{""}},
		{"x\n", []string{"x"}, []string{"\n"}}, {"x\r\n", []string{"x"}, []string{"\r\n"}},
		{"\n\n", []string{"", ""}, []string{"\n", "\n"}},
		{"\uFEFFя\r\n \t\n\uFEFF尾", []string{"я", " \t", "\uFEFF尾"}, []string{"\r\n", "\n", ""}},
		{"\uFEFF\uFEFFx", []string{"\uFEFFx"}, []string{""}}, {"x\ry", []string{"x\ry"}, []string{""}},
	} {
		t.Run(fmt.Sprintf("%q", tc.source), func(t *testing.T) {
			lines := splitLines([]byte(tc.source))
			if lines == nil || len(lines) != len(tc.raw) {
				t.Fatalf("Неверное число строк: %+v", lines)
			}
			for i, line := range lines {
				if line.Number != i+1 || line.Raw != tc.raw[i] || string(line.LineEnding) != tc.eol[i] {
					t.Fatalf("Изменена строка: %+v", line)
				}
			}
		})
	}
	doc := model.Document{}
	recordByteFacts(&doc, nil)
	if *doc.SHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" || *doc.ByteLength != 0 || *doc.HasBOM {
		t.Fatal("Неверные факты пустого источника")
	}
}

func TestPrepareInputReadFailureDiscardsPartialFacts(t *testing.T) {
	readErr := errors.New("обрыв потока")
	for _, partial := range []string{"", "\uFEFF@dsl-version 1.2\n\xff\r"} {
		t.Run(fmt.Sprintf("%q", partial), func(t *testing.T) {
			p := inputParser(t, "")
			p.reader = io.MultiReader(strings.NewReader(partial), &parserCountingReader{err: readErr})
			recordByteFacts(&p.result.Document, []byte("stale"))
			lines, ready, err := p.prepareInput()
			if err != nil || ready || len(lines) != 0 || len(p.result.Diagnostics) != 1 {
				t.Fatalf("Ошибка чтения не стала IO001: %v", err)
			}
			doc, diagnostic := p.result.Document, p.result.Diagnostics[0]
			if diagnostic.DiagnosticCode != diagnostics.IO001 || !strings.Contains(diagnostic.Message, readErr.Error()) || !diagnostic.Fatal || diagnostic.Source != "current" {
				t.Fatalf("Неверная диагностика: %+v", diagnostic)
			}
			if doc.ByteLength != nil || doc.SHA256 != nil || doc.HasBOM != nil || doc.Encoding != nil || doc.LineCount != nil || doc.DSLVersion != nil {
				t.Fatal("Неполный источник получил факты полного документа")
			}
		})
	}
}

func TestInputChecksUseRegistryAndIndependentReaders(t *testing.T) {
	for _, fail := range []bool{false, true} {
		p := inputParser(t, "\uFEFF\xff\r")
		registry := inputRegistry{}
		var called []diagnostics.Code
		checkErr := errors.New("сбой проверки")
		for _, code := range []diagnostics.Code{diagnostics.P001, diagnostics.P002} {
			description, _ := p.diagnostics.Lookup(code)
			registry[code] = inputCheck{Diagnostic: description, check: func(reader io.Reader) (bool, error) {
				called = append(called, code)
				source, err := io.ReadAll(reader)
				if err != nil || string(source) != "\uFEFF\xff\r" {
					t.Fatal("Проверка не получила точные байты с начала")
				}
				if fail && code == diagnostics.P001 {
					return false, checkErr
				}
				return true, nil
			}}
		}
		p.diagnostics = registry
		_, ready, err := p.prepareInput()
		if ready || (fail && !errors.Is(err, checkErr)) || (!fail && err != nil) || !slices.Equal(called, []diagnostics.Code{diagnostics.P001, diagnostics.P002}) {
			t.Fatalf("Проверки зависимы: %v, %v", called, err)
		}
	}
}

func TestInputMissingChecksAreConfigurationErrors(t *testing.T) {
	for _, code := range []diagnostics.Code{diagnostics.IO001, diagnostics.P001, diagnostics.P002, diagnostics.P013} {
		for _, missing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/missing=%t", code, missing), func(t *testing.T) {
				p := inputParser(t, "@dsl-version 1.2")
				description, _ := p.diagnostics.Lookup(code)
				registry := inputRegistry{code: inputCheck{Diagnostic: description}}
				if missing {
					registry[code] = nil
				}
				p.diagnostics = registry
				_, ready, err := p.prepareInput()
				if ready || !errors.Is(err, diagnostics.ErrCheckUnavailable) || len(p.result.Diagnostics) != 0 {
					t.Fatalf("Конфигурация стала ошибкой входа: %v", err)
				}
			})
		}
	}
}

func TestSelectGrammarRequiresRegisteredRolesAndFunctions(t *testing.T) {
	for _, fault := range []string{"none", "map", "version absent", "empty map", "enter absent", "leave absent", "enter nil", "leave nil", "enter ID", "leave ID"} {
		t.Run(fault, func(t *testing.T) {
			p := inputParser(t, "@dsl-version future")
			enter, leave := grammar.AssertionID("enter"), grammar.AssertionID("leave")
			checks := map[grammar.AssertionID]grammar.AssertionFunc{enter: func(grammar.AssertionInput) bool { return true }, leave: func(grammar.AssertionInput) bool { return false }}
			switch fault {
			case "map":
				checks = nil
			case "empty map":
				checks = map[grammar.AssertionID]grammar.AssertionFunc{}
			case "enter absent":
				delete(checks, enter)
			case "leave absent":
				delete(checks, leave)
			case "enter nil":
				checks[enter] = nil
			case "leave nil":
				checks[leave] = nil
			case "enter ID":
				enter = ""
			case "leave ID":
				leave = ""
			}
			var called []string
			p.grammars.Register("future", []grammar.DetectionFunc{func(grammar.GrammarContext) bool { called = append(called, "d1"); return true }, func(grammar.GrammarContext) bool { called = append(called, "d2"); return true }}, []grammar.OrchestrationFunc{func(grammar.GrammarContext) bool { called = append(called, "o1"); return true }}, nil, nil, enter, leave)
			p.assertionFuncs["future"] = checks
			if fault == "version absent" {
				delete(p.assertionFuncs, "future")
			}
			_, ready, err := p.prepareInput()
			if (err == nil) != (fault == "none") || ready != (fault == "none") || len(p.result.Diagnostics) != 0 {
				t.Fatalf("Неверный выбор грамматики: ready=%t, err=%v", ready, err)
			}
			if !ready {
				return
			}
			if p.enterParentID != enter || p.leaveParentID != leave || !p.assertions[enter](grammar.AssertionInput{}) || p.assertions[leave](grammar.AssertionInput{}) || *p.result.Document.DSLVersion != "future" {
				t.Fatal("Неверные роли или версия")
			}
			for _, detect := range p.detections {
				detect(nil)
			}
			for _, orchestrate := range p.orchestrators {
				orchestrate(nil)
			}
			if !slices.Equal(called, []string{"d1", "d2", "o1"}) {
				t.Fatalf("Неверные действия: %v", called)
			}
		})
	}
}

func TestReadVersionUsesRegisteredCheck(t *testing.T) {
	for _, checkErr := range []error{nil, errors.New("сбой проверки версии")} {
		p := inputParser(t, "\uFEFF@dsl-version 1.2\r\ntail")
		description, _ := p.diagnostics.Lookup(diagnostics.P013)
		called := false
		p.diagnostics = inputRegistry{diagnostics.P013: inputCheck{Diagnostic: description, check: func(reader io.Reader) (bool, error) {
			called = true
			source, err := io.ReadAll(reader)
			if err != nil || string(source) != "\uFEFF@dsl-version 1.2\r\n" {
				t.Fatal("P013 не получила точную первую строку")
			}
			return true, checkErr
		}}}
		_, ready, err := p.prepareInput()
		if !called || ready || !errors.Is(err, checkErr) {
			t.Fatalf("Результат зарегистрированной P013 проигнорирован: %v", err)
		}
		if checkErr == nil && (len(p.result.Diagnostics) != 1 || p.result.Diagnostics[0].DiagnosticCode != diagnostics.P013) {
			t.Fatal("Не сохранена зарегистрированная P013")
		}
	}
}

func inputParser(t *testing.T, source string) *Parser {
	t.Helper()
	registry := grammar.NewRegistry()
	checks := gr12.BuiltinGr12(registry)
	p := New(strings.NewReader(source), parserInitialResult(t), registry, diagnostics.NewRegistry(), map[string]map[grammar.AssertionID]grammar.AssertionFunc{"1.2": checks})
	if err := p.prepareResult(); err != nil {
		t.Fatal(err)
	}
	return p
}

type inputRegistry map[diagnostics.Code]diagnostics.Diagnostic

func (r inputRegistry) Lookup(code diagnostics.Code) (diagnostics.Diagnostic, bool) {
	if description, found := r[code]; found {
		return description, description != nil
	}
	return diagnostics.NewRegistry().Lookup(code)
}

type inputCheck struct {
	diagnostics.Diagnostic
	check diagnostics.CheckFunc
}

func (d inputCheck) HasCheck() bool { return d.check != nil }

func (d inputCheck) Check(reader io.Reader) (bool, error) {
	if d.check == nil {
		return false, diagnostics.ErrCheckUnavailable
	}
	return d.check(reader)
}
