package parser

import (
	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
)

// filterMultifillP012 исключает только ложную P012 для нескольких ответов
// в содержимом multifill DSL 1.2. Весь диагностируемый элемент должен быть
// корректен; наличие допустимой цепочки не скрывает другие ошибочные скобки.
func filterMultifillP012(input diagnosticRuleInput) (diagnosticDecision, error) {
	if input.Version != "1.2" || input.Occurrence.DiagnosticCode != diagnostics.P012 || input.Line.LineType != model.LineTypeContent {
		return diagnosticContinue, nil
	}
	block := input.OpenBlock
	if block == nil || block.LineType != model.LineTypeBlockStart || block.Number < 1 || block.Number >= input.Line.Number {
		return diagnosticContinue, nil
	}
	multifill := false
	for _, element := range block.Elements {
		if element.ElementType == model.ElementTypeTag && element.Value != nil && *element.Value == "multifill" {
			multifill = true
			break
		}
	}
	if !multifill {
		return diagnosticContinue, nil
	}
	element, location := input.Element, input.Occurrence.Location
	if element == nil || element.ElementType != model.ElementTypeContent || location == nil ||
		location.Start.Line != input.Line.Number || location.End.Line != input.Line.Number ||
		location.Start.Column != element.Start || location.End.Column != element.End {
		return diagnosticContinue, nil
	}
	source := []rune(input.Line.Raw)
	start, end := element.Start-1, element.End-1
	if start < 0 || end <= start || end > len(source) || element.Raw != string(source[start:end]) {
		return diagnosticContinue, nil
	}
	if validMultifillAlternatives(source, start, end) {
		return diagnosticSuppress, nil
	}
	return diagnosticContinue, nil
}

// validMultifillAlternatives проверяет диапазон [start,end) в кодовых точках.
// true требует хотя бы одной цепочки из нескольких непустых ответов и полного
// отсутствия лишних скобок. Экранирование учитывается и перед началом диапазона.
func validMultifillAlternatives(source []rune, start, end int) bool {
	escaped, alternatives := false, false
	for i := start - 1; i >= 0 && source[i] == '\\'; i-- {
		escaped = !escaped
	}
	for i := start; i < end; {
		if escaped {
			escaped = false
			if source[i] == '_' {
				for i < end && source[i] == '_' {
					i++
				}
			} else {
				i++
			}
			continue
		}
		switch source[i] {
		case '\\':
			escaped = true
			i++
		case '_':
			first := i
			for i < end && source[i] == '_' {
				i++
			}
			if i-first != 5 || i == end || source[i] != '{' {
				continue
			}
			answers := 0
			for i < end && source[i] == '{' {
				next, ok := multifillAnswerEnd(source, i, end)
				if !ok {
					return false
				}
				i = next
				answers++
			}
			alternatives = alternatives || answers > 1
		case '{', '}':
			return false
		default:
			i++
		}
	}
	return alternatives
}

// multifillAnswerEnd сохраняет существующее правило непустого ответа:
// пробельное содержимое допустимо, вложенная неэкранированная { — нет.
func multifillAnswerEnd(source []rune, opening, end int) (int, bool) {
	for i := opening + 1; i < end; i++ {
		switch source[i] {
		case '\\':
			i++
		case '{':
			return 0, false
		case '}':
			return i + 1, i > opening+1
		}
	}
	return 0, false
}
