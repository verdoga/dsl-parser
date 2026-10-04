package gr12

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestDetectLineCountsSourceCharacters(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   map[string]int
	}{
		{name: "empty"},
		{name: "Unicode without counted characters", source: "Привет世界🌍е\u0301"},
		{name: "other whitespace and lookalikes", source: "\u00a0\u2003\u2009\u3000＠｛｝＂＿"},
		{name: "edge and internal whitespace", source: " \tПривет \tмир  ", want: map[string]int{
			"spaces": 4, "tabs": 2,
		}},
		{name: "all counted characters", source: " \t@@{{}}\"\"___", want: map[string]int{
			"spaces": 1, "tabs": 1, "at-signs": 2, "open-braces": 2,
			"close-braces": 2, "quotes": 2, "underscores": 3,
		}},
		{name: "escaped characters are included in totals", source: `\@\{\}\"\_____`, want: map[string]int{
			"at-signs": 1, "open-braces": 1, "close-braces": 1, "quotes": 1, "underscores": 5,
			"escaped-open-braces": 1, "escaped-close-braces": 1, "escaped-quotes": 1, "escaped-underscores": 1,
		}},
		{name: "quoted resource path has no special counting rules", source: `@resource-dir "C:\{one\},_"`, want: map[string]int{
			"spaces": 1, "at-signs": 1, "quotes": 2, "open-braces": 1, "close-braces": 1,
			"escaped-open-braces": 1, "escaped-close-braces": 1, "underscores": 1,
		}},
		{name: "mixed escaped and unescaped braces", source: `{\{}\}`, want: map[string]int{
			"open-braces": 2, "close-braces": 2, "escaped-open-braces": 1, "escaped-close-braces": 1,
		}},
		{name: "trailing backslash", source: `{\`, want: map[string]int{"open-braces": 1}},
		{name: "backslashes only", source: `\\\`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context := grammar.NewContext(test.source)
			if !detectLine(context) {
				t.Fatal("Подсчёт не завершён")
			}
			assertDetectionCounts(t, context, test.want)
			if context.String() != test.source {
				t.Fatal("Исходная строка изменена")
			}
		})
	}
}

func TestDetectLineUsesImmediateBackslashParity(t *testing.T) {
	symbols := []struct {
		symbol  string
		total   string
		escaped string
	}{
		{"{", "open-braces", "escaped-open-braces"},
		{"}", "close-braces", "escaped-close-braces"},
		{`"`, "quotes", "escaped-quotes"},
		{"_", "underscores", "escaped-underscores"},
	}
	for _, symbol := range symbols {
		for _, test := range []struct{ backslashes, escaped int }{
			{0, 0}, {1, 1}, {2, 0}, {3, 1}, {4, 0}, {5, 1}, {64, 0}, {65, 1},
		} {
			t.Run(fmt.Sprintf("%s/%d", symbol.total, test.backslashes), func(t *testing.T) {
				source := strings.Repeat(`\`, test.backslashes) + symbol.symbol + symbol.symbol
				context := grammar.NewContext(source)
				if !detectLine(context) {
					t.Fatal("Подсчёт не завершён")
				}
				assertDetectionCounts(t, context, map[string]int{symbol.total: 2, symbol.escaped: test.escaped})
			})
		}
	}
}

func TestDetectLineResetsEscapingAfterAnyOtherCharacter(t *testing.T) {
	for _, separator := range []string{"a", "Ж", "🌍", " ", "\t", "@", "\u0301"} {
		t.Run(fmt.Sprintf("%q", separator), func(t *testing.T) {
			context := grammar.NewContext(`\` + separator + `{}"_`)
			want := map[string]int{"open-braces": 1, "close-braces": 1, "quotes": 1, "underscores": 1}
			switch separator {
			case " ":
				want["spaces"] = 1
			case "\t":
				want["tabs"] = 1
			case "@":
				want["at-signs"] = 1
			}
			if !detectLine(context) {
				t.Fatal("Подсчёт не завершён")
			}
			assertDetectionCounts(t, context, want)
		})
	}
}

func TestDetectLineReplacesPreviousCountsIncludingZeros(t *testing.T) {
	for _, source := range []string{"", `@{\_`} {
		t.Run(source, func(t *testing.T) {
			context := grammar.NewContext(source)
			for _, kind := range []string{
				"spaces", "tabs", "at-signs", "open-braces", "close-braces", "escaped-open-braces",
				"escaped-close-braces", "quotes", "escaped-quotes", "underscores", "escaped-underscores",
			} {
				context.AddDetection(kind, 99)
			}
			context.AddDetection("other", 27)
			want := map[string]int{}
			if source != "" {
				want = map[string]int{"at-signs": 1, "open-braces": 1, "underscores": 1, "escaped-underscores": 1}
			}
			for attempt := 0; attempt < 2; attempt++ {
				if !detectLine(context) {
					t.Fatal("Подсчёт не завершён")
				}
				assertDetectionCounts(t, context, want)
				if got, found := context.Detection("other"); !found || got != 27 {
					t.Fatal("Посторонний признак изменён")
				}
			}
		})
	}
}

func TestDetectLinePreservesSourceLineElementsAndDiagnostics(t *testing.T) {
	context := grammar.NewContext(`@\{`)
	parent, value := 2, "текст"
	location := model.Location{Start: model.Position{Line: 3, Column: 1}, End: model.Position{Line: 3, Column: 6}}
	context.SetLine(model.Line{
		Number: 3, LineType: model.LineTypeContent, NestingLevel: 1, ParentLine: &parent,
		Raw: "текст", LineEnding: model.LineEndingCRLF, HasErrors: true,
		Elements: []model.Element{{ElementType: model.ElementTypeContent, Raw: "текст", Value: &value,
			Start: 1, End: 6, ErrorIDs: []string{"d1"}}},
	})
	context.AddDiagnostic(model.Diagnostic{
		ID: "d1", Source: "parser", DiagnosticCode: diagnostics.P012,
		SeverityLevel: diagnostics.SeverityError, Message: "сохранённая диагностика",
		DiagnosticScope: diagnostics.ScopeElement, Location: &location,
		RelatedLocations: []model.Location{{Start: model.Position{Line: 2, Column: 1}, End: model.Position{Line: 2, Column: 2}}},
	})
	before := detectionContextSnapshot(t, context)
	if !detectLine(context) {
		t.Fatal("Подсчёт не завершён")
	}
	assertDetectionCounts(t, context, map[string]int{"at-signs": 1, "open-braces": 1, "escaped-open-braces": 1})
	if after := detectionContextSnapshot(t, context); after != before {
		t.Fatalf("Изменены данные контекста:\nдо: %s\nпосле: %s", before, after)
	}
}

func TestDetectLineKeepsContextsIndependent(t *testing.T) {
	first, second := grammar.NewContext(`\{`), grammar.NewContext(`@@__`)
	firstWant := map[string]int{"open-braces": 1, "escaped-open-braces": 1}
	secondWant := map[string]int{"at-signs": 2, "underscores": 2}
	if !detectLine(first) || !detectLine(second) {
		t.Fatal("Подсчёт не завершён")
	}
	assertDetectionCounts(t, first, firstWant)
	assertDetectionCounts(t, second, secondWant)
	first.AddDetection("at-signs", 99)
	if !detectLine(first) {
		t.Fatal("Повторный подсчёт не завершён")
	}
	assertDetectionCounts(t, first, firstWant)
	assertDetectionCounts(t, second, secondWant)
}

func TestDetectLineWithoutContextReturnsFalse(t *testing.T) {
	if detectLine(nil) {
		t.Fatal("Подсчёт без контекста не должен завершаться успешно")
	}
}

func assertDetectionCounts(t *testing.T, context grammar.GrammarContext, want map[string]int) {
	t.Helper()
	for _, kind := range []string{
		"spaces", "tabs", "at-signs", "open-braces", "close-braces", "escaped-open-braces",
		"escaped-close-braces", "quotes", "escaped-quotes", "underscores", "escaped-underscores",
	} {
		got, found := context.Detection(kind)
		if !found || got != want[kind] {
			t.Errorf("Detection(%q) = (%d, %t), требуется (%d, true)", kind, got, found, want[kind])
		}
	}
}

func detectionContextSnapshot(t *testing.T, context grammar.GrammarContext) string {
	t.Helper()
	encoded, err := json.Marshal(struct {
		Source      string
		Line        model.Line
		Diagnostics []model.Diagnostic
	}{context.String(), context.Line(), context.Diagnostics()})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
