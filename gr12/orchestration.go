package gr12

import "github.com/verdoga/dsl-parser/grammar"

// orchestrateLine выбирает функции определения типа и разбора для этой строки.
// Порядок их вызовов — внутреннее решение грамматики версии 1.2.
// true означает завершённую обработку, включая диагностированную ошибку.
// false означает отсутствие контекста, незавершённое детектирование либо
// невозможность закончить классификацию или разбор. Нулевой счётчик —
// готовый результат детектирования, а не отсутствие признака.
func orchestrateLine(context grammar.GrammarContext) bool {
	if context == nil {
		return false
	}
	for _, kind := range [...]string{
		spacesKind, tabsKind, atSignsKind, openBracesKind, closeBracesKind,
		escapedOpenBracesKind, escapedCloseBracesKind, quotesKind,
		escapedQuotesKind, underscoresKind, escapedUnderscoresKind,
	} {
		if _, found := context.Detection(kind); !found {
			return false
		}
	}

	// Счётчики учитывают и текстовые символы, поэтому сами по себе
	// не устанавливают тип: положение и форму определяет классификация.
	if !classifyLine(context) {
		return false
	}
	return parseLine(context)
}
