package gr12

import (
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

// parseContent сохраняет непустую строку одним элементом content.
// Текст не разбивается на ячейки, варианты, плейсхолдеры или HTML.
// Скобки проверяются с учётом экранирования и синтаксической формы ответа.
// Исключаются только краевые U+0020 и TAB; экранирование сохраняется.
func parseContent(context grammar.GrammarContext) {
	if context == nil {
		return
	}
	source := []rune(context.String())
	start, end := 0, len(source)
	for start < end && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	if start < end {
		value := string(source[start:end])
		addElement(context, model.ElementTypeContent, start+1, end+1, &value)
		checkTextBraces(context, start+1, end+1, true)
	}
}

// hasStrayBrace ищет неэкранированную скобку в диапазоне [start,end).
// Индексы измеряются в кодовых точках. Экранирование перед началом диапазона
// учитывается; экранированная последовательность подчёркиваний остаётся текстом.
func hasStrayBrace(source []rune, start, end int, allowAnswers bool) bool {
	escaped := false
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
			if allowAnswers && i-first == 5 && i < end && source[i] == '{' {
				next, ok := placeholderAnswerEnd(source, i, end)
				if !ok {
					return true
				}
				i = next
			}
		case '{', '}':
			return true
		default:
			i++
		}
	}
	return false
}

// placeholderAnswerEnd находит конец непустого ответа после открывающей скобки.
// next — индекс после закрывающей скобки. Вложенное неэкранированное открытие
// и отсутствие закрытия означают false; семантика значения здесь не проверяется.
func placeholderAnswerEnd(source []rune, opening, end int) (next int, ok bool) {
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
