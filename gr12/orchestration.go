package gr12

import (
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

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

	// Заданный вызывающим кодом текстовый тип сохраняется: содержимое
	// не классифицируется повторно как DSL. Открытые блоки здесь не нужны.
	kind := context.Line().LineType
	if kind != model.LineTypeContent && kind != model.LineTypeBlank && !classifyLine(context) {
		return false
	}
	return parseLine(context)
}
