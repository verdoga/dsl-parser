package gr12

import (
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
)

func TestContentBraceSyntax(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		invalid bool
	}{
		{"Use {x} here.", true}, {"Text } @note Text", true},
		{"Ёж🌍 {x}", true}, {`Use \{x\} here.`, false},
		{`Use \\{x} here.`, true}, {`Use \\\{x\} here.`, false},
		{"She _____{lives} here.", false}, {"_____{A} and _____{B}", false},
		{`_____{a\}b}`, false}, {`_____{a\{b\}}`, false}, {`_____{a\\}`, false},
		{`_____{a\}`, true}, {"_____{a{b}}", true}, {"_____{a", true},
		{"_____{}", true}, {"_____{ }", false}, {"______{a}", true},
		{"____{a}", true}, {"_____ {a}", true}, {`\_____{a}`, true},
		{`\______{a}`, true}, {`\_____\{a\}`, false},
		{`\\_____{a}`, false}, {"_____{a} {b}", true}, {"_____{a}}", true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			c := newLineTestContext(t, " \t"+tc.raw+" \t")
			if !detectLine(c) || !orchestrateLine(c) {
				t.Fatal("Разбор не завершён")
			}
			line := c.Line()
			if line.LineType != model.LineTypeContent || len(line.Elements) != 1 {
				t.Fatalf("Изменено содержимое: %+v", line)
			}
			assertOperandElement(t, line.Elements[0], operandWant(model.ElementTypeContent, tc.raw, tc.raw, 3))
			if !tc.invalid {
				if len(c.Diagnostics()) != 0 {
					t.Fatalf("Лишние диагностики: %+v", c.Diagnostics())
				}
				return
			}
			if len(c.Diagnostics()) != 1 {
				t.Fatalf("Требуется одна P012: %+v", c.Diagnostics())
			}
			d := c.Diagnostics()[0]
			want := model.Location{Start: model.Position{Line: line.Number, Column: 3}, End: model.Position{Line: line.Number, Column: 3 + utf8.RuneCountInString(tc.raw)}}
			if d.DiagnosticCode != diagnostics.P012 || d.DiagnosticScope != diagnostics.ScopeElement || d.Location == nil || *d.Location != want {
				t.Fatalf("Неверная диагностика: %+v", d)
			}
			if c.Diagnostics()[0].Fatal {
				t.Fatal("P012 не должна быть фатальной")
			}
		})
	}
}

func TestOrchestrationHonorsPresetTextType(t *testing.T) {
	for _, raw := range []string{"@unknown", "# Title", "}"} {
		c := newLineTestContext(t, raw)
		line := c.Line()
		line.LineType = model.LineTypeContent
		c.SetLine(line)
		if !detectLine(c) || !orchestrateLine(c) {
			t.Fatal("Разбор не завершён")
		}
		if c.Line().LineType != model.LineTypeContent || len(c.Line().Elements) != 1 || c.Line().Elements[0].ElementType != model.ElementTypeContent {
			t.Fatalf("Содержимое повторно распознано как DSL: %+v", c.Line())
		}
	}
}
