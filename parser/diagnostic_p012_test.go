package parser

import (
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
)

func TestFilterMultifillP012ChecksWholeElement(t *testing.T) {
	for _, tc := range []struct {
		raw      string
		suppress bool
	}{
		{"_____{A}{B}", true},
		{"_____{A}{B}{C}", true},
		{"_____{B: No, we went by bus.}{No, we took the bus.}", true},
		{"_____{A}{B} and _____{C}{D}", true},
		{"_____{A} and _____{B}{C}", true},
		{"_____{A}{B} and _____{C}", true},
		{"Ёж🌍 _____{А}{Б} Málaga", true},
		{"_____{ }{B}", true},
		{`_____{a\}b}{c}`, true},
		{`_____{a\{b\}}{c}`, true},
		{`_____{a\\}{b}`, true},
		{`_____{A}{B\}C}`, true},
		{`\{текст\} _____{A}{B}`, true},
		{`\_____\{a\} _____{B}{C}`, true},
		{`\\_____{A}{B}`, true},
		{`_____{A}{B}\`, true},
		{"", false},
		{"plain text", false},
		{"_____", false},
		{"_____{A}", false},
		{"_____{A} and _____{B}", false},
		{`_____{A}\{B\}`, false},
		{"_____{A}{}", false},
		{"_____{}{B}", false},
		{"_____{A}{B", false},
		{"_____{A}{B{C}}", false},
		{"_____{A{B}}{C}", false},
		{"_____{A}}", false},
		{"_____{A} {B}", false},
		{"_____{A}\t{B}", false},
		{"_____{A}{B} текст {", false},
		{"{x} _____{A}{B}", false},
		{"_____{A}{B} }", false},
		{"_____{A}{B} _____{}", false},
		{"_____{A}{B} _____{C", false},
		{"____{A}{B}", false},
		{"______{A}{B}", false},
		{"_____ {A}{B}", false},
		{"_____{A}{B} ______{C}", false},
		{`\_____{A}{B}`, false},
		{`\\\_____{A}{B}`, false},
		{`_____{A}{B\}`, false},
		{`_____{A}{B} \\{x}`, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			input := multifillP012TestInput(tc.raw)
			before := parserSnapshot(t, input)
			got, err := filterMultifillP012(input)
			want := diagnosticContinue
			if tc.suppress {
				want = diagnosticSuppress
			}
			if err != nil || got != want {
				t.Fatalf("Решение %d, ошибка %v; требуется %d", got, err, want)
			}
			if parserSnapshot(t, input) != before {
				t.Fatal("Правило изменило входные данные")
			}
		})
	}
}

func TestFilterMultifillP012RequiresMatchingContext(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*diagnosticRuleInput)
	}{
		{"other version", func(i *diagnosticRuleInput) { i.Version = "2.0" }},
		{"other code", func(i *diagnosticRuleInput) { i.Occurrence.DiagnosticCode = diagnostics.P008 }},
		{"heading", func(i *diagnosticRuleInput) { i.Line.LineType = model.LineTypeHeading }},
		{"declaration", func(i *diagnosticRuleInput) { i.Line.LineType = model.LineTypeTag }},
		{"block opening", func(i *diagnosticRuleInput) { i.Line.LineType = model.LineTypeBlockStart }},
		{"block closing", func(i *diagnosticRuleInput) { i.Line.LineType = model.LineTypeBlockEnd }},
		{"no block", func(i *diagnosticRuleInput) { i.OpenBlock = nil }},
		{"other nearest block", func(i *diagnosticRuleInput) { *i.OpenBlock.Elements[0].Value = "text" }},
		{"only logical parent is multifill", func(i *diagnosticRuleInput) { i.OpenBlock = nil }},
		{"not a block opening", func(i *diagnosticRuleInput) { i.OpenBlock.LineType = model.LineTypeTag }},
		{"invalid block number", func(i *diagnosticRuleInput) { i.OpenBlock.Number = 0 }},
		{"current line is opening", func(i *diagnosticRuleInput) { i.OpenBlock.Number = i.Line.Number }},
		{"missing tag", func(i *diagnosticRuleInput) { i.OpenBlock.Elements = nil }},
		{"missing tag value", func(i *diagnosticRuleInput) { i.OpenBlock.Elements[0].Value = nil }},
		{"wrong marker type", func(i *diagnosticRuleInput) { i.OpenBlock.Elements[0].ElementType = model.ElementTypeContent }},
		{"no element", func(i *diagnosticRuleInput) { i.Element = nil }},
		{"other element type", func(i *diagnosticRuleInput) { i.Element.ElementType = model.ElementTypeTitle }},
		{"no location", func(i *diagnosticRuleInput) { i.Occurrence.Location = nil }},
		{"other start line", func(i *diagnosticRuleInput) { i.Occurrence.Location.Start.Line-- }},
		{"other end line", func(i *diagnosticRuleInput) { i.Occurrence.Location.End.Line++ }},
		{"different start", func(i *diagnosticRuleInput) { i.Occurrence.Location.Start.Column++ }},
		{"different end", func(i *diagnosticRuleInput) { i.Occurrence.Location.End.Column-- }},
		{"invalid start", func(i *diagnosticRuleInput) { i.Element.Start = 0; i.Occurrence.Location.Start.Column = 0 }},
		{"outside source", func(i *diagnosticRuleInput) { i.Element.End = 999; i.Occurrence.Location.End.Column = 999 }},
		{"reversed range", func(i *diagnosticRuleInput) { i.Element.End = 1; i.Occurrence.Location.End.Column = 1 }},
		{"empty range", func(i *diagnosticRuleInput) {
			i.Element.End = i.Element.Start
			i.Occurrence.Location.End.Column = i.Element.Start
		}},
		{"element differs from source", func(i *diagnosticRuleInput) { i.Element.Raw = "_____{other}{answer}" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := multifillP012TestInput("_____{A}{B}")
			tc.change(&input)
			before := parserSnapshot(t, input)
			got, err := filterMultifillP012(input)
			if err != nil || got != diagnosticContinue {
				t.Fatalf("P012 подавлена вне подходящего контекста: %d, %v", got, err)
			}
			if parserSnapshot(t, input) != before {
				t.Fatal("Неподходящий контекст изменён")
			}
		})
	}
}

func TestFilterMultifillP012UsesRawAndNearestBlock(t *testing.T) {
	input := multifillP012TestInput("_____{A}{B}")
	input.OpenBlock.Elements[0].Raw = "@MuLtIfIlL"
	*input.Parent.Elements[0].Value = "text"
	*input.Element.Value = "{не проверять семантическое значение}"
	if got, err := filterMultifillP012(input); err != nil || got != diagnosticSuppress {
		t.Fatalf("Не использованы исходный текст и нормализованный тег ближайшего блока: %d, %v", got, err)
	}
}

func TestFilterMultifillP012PreservesEscapingBeforeElement(t *testing.T) {
	for _, tc := range []struct {
		prefix string
		want   diagnosticDecision
	}{
		{`\`, diagnosticContinue},
		{`\\`, diagnosticSuppress},
		{`\\\`, diagnosticContinue},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			input := multifillP012TestInput("_____{A}{B}")
			input.Line.Raw = tc.prefix + input.Element.Raw
			input.Element.Start = utf8.RuneCountInString(tc.prefix) + 1
			input.Element.End = utf8.RuneCountInString(input.Line.Raw) + 1
			input.Line.Elements = []model.Element{*input.Element}
			input.Occurrence.Location.Start.Column = input.Element.Start
			input.Occurrence.Location.End.Column = input.Element.End
			if got, err := filterMultifillP012(input); err != nil || got != tc.want {
				t.Fatalf("Экранирование перед диапазоном потеряно: %d, %v", got, err)
			}
		})
	}
}

func multifillP012TestInput(raw string) diagnosticRuleInput {
	tag, brace := "multifill", "{"
	block := model.Line{Number: 2, LineType: model.LineTypeBlockStart, Raw: "@multifill {",
		Elements: []model.Element{
			{ElementType: model.ElementTypeTag, Raw: "@multifill", Value: &tag, Start: 1, End: 11},
			{ElementType: model.ElementTypeBlockOpen, Raw: brace, Value: &brace, Start: 12, End: 13},
		}}
	parent := block.Number
	line := model.Line{Number: 3, LineType: model.LineTypeContent, Raw: " \t" + raw + " \t", ParentLine: &parent,
		Elements: []model.Element{{ElementType: model.ElementTypeContent, Raw: raw, Value: &raw, Start: 3, End: 3 + utf8.RuneCountInString(raw)}}}
	diagnostic := model.Diagnostic{DiagnosticCode: diagnostics.P012, DiagnosticScope: diagnostics.ScopeElement,
		Location: &model.Location{Start: model.Position{Line: 3, Column: 3}, End: model.Position{Line: 3, Column: line.Elements[0].End}}}
	return newDiagnosticRuleInput("1.2", diagnostic, line, &block, &block)
}
