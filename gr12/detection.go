package gr12

import "github.com/verdoga/dsl-parser/grammar"

// spacesKind обозначает число обычных пробелов U+0020 в исходной строке.
const spacesKind = "spaces"

// tabsKind обозначает число табуляций в исходной строке.
const tabsKind = "tabs"

// atSignsKind обозначает число символов @ в исходной строке.
const atSignsKind = "at-signs"

// openBracesKind обозначает число открывающих фигурных скобок в строке.
const openBracesKind = "open-braces"

// closeBracesKind обозначает число закрывающих фигурных скобок в строке.
const closeBracesKind = "close-braces"

// escapedOpenBracesKind обозначает число экранированных открывающих скобок.
const escapedOpenBracesKind = "escaped-open-braces"

// escapedCloseBracesKind обозначает число экранированных закрывающих скобок.
const escapedCloseBracesKind = "escaped-close-braces"

// quotesKind обозначает число двойных кавычек в исходной строке.
const quotesKind = "quotes"

// escapedQuotesKind обозначает число экранированных двойных кавычек.
const escapedQuotesKind = "escaped-quotes"

// underscoresKind обозначает число символов подчёркивания в строке.
const underscoresKind = "underscores"

// escapedUnderscoresKind обозначает число экранированных подчёркиваний.
const escapedUnderscoresKind = "escaped-underscores"

// detectLine считает признаки и записывает их через GrammarContext.AddDetection.
// Экранированным считается символ после непарной обратной косой черты.
// true означает завершённый подсчёт, false — невозможность выполнить его.
func detectLine(context grammar.GrammarContext) bool {
	if context == nil {
		return false
	}

	var spaces, tabs, atSigns, openBraces, closeBraces int
	var escapedOpenBraces, escapedCloseBraces, quotes, escapedQuotes int
	var underscores, escapedUnderscores int
	escaped := false
	for _, symbol := range context.String() {
		if symbol == '\\' {
			escaped = !escaped
			continue
		}
		switch symbol {
		case ' ':
			spaces++
		case '\t':
			tabs++
		case '@':
			atSigns++
		case '{':
			openBraces++
			if escaped {
				escapedOpenBraces++
			}
		case '}':
			closeBraces++
			if escaped {
				escapedCloseBraces++
			}
		case '"':
			quotes++
			if escaped {
				escapedQuotes++
			}
		case '_':
			underscores++
			if escaped {
				escapedUnderscores++
			}
		}
		escaped = false
	}

	context.AddDetection(spacesKind, spaces)
	context.AddDetection(tabsKind, tabs)
	context.AddDetection(atSignsKind, atSigns)
	context.AddDetection(openBracesKind, openBraces)
	context.AddDetection(closeBracesKind, closeBraces)
	context.AddDetection(escapedOpenBracesKind, escapedOpenBraces)
	context.AddDetection(escapedCloseBracesKind, escapedCloseBraces)
	context.AddDetection(quotesKind, quotes)
	context.AddDetection(escapedQuotesKind, escapedQuotes)
	context.AddDetection(underscoresKind, underscores)
	context.AddDetection(escapedUnderscoresKind, escapedUnderscores)
	return true
}
