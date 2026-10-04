package gr12

import (
	"strings"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

// parseTag распознаёт одно из 34 имён тегов и выбирает допустимую форму.
// Неизвестное имя получает P003; исходное написание сохраняется.
// Имя с @ записывается как unparsed; остаток неизвестного объявления
// остаётся в исходнике строки и не передаётся обработчику известного тега.
func parseTag(context grammar.GrammarContext) {
	if context == nil {
		return
	}
	source := []rune(context.String())
	start := 0
	for start < len(source) && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	if start == len(source) || source[start] != '@' {
		return
	}
	name, next, _ := readTagName(context)
	canonical := strings.ToLower(name)
	var parse func(grammar.GrammarContext)
	var kind model.ElementType
	switch canonical {
	case "endtask", "speaking", "newpage":
		parse = parseBareTag
	case "dsl-version":
		kind = model.ElementTypeVersion
	case "document-id", "task", "include":
		kind = model.ElementTypeIdentifier
	case "order":
		kind = model.ElementTypeNumber
	case "section", "variant":
		kind = model.ElementTypeName
	case "header", "step":
		kind = model.ElementTypeTitle
	case "question":
		kind = model.ElementTypeContent
	case "editor", "instr", "hint", "answer":
		parse = parsePlainTextOrBlock
	case "example", "wordlist", "note", "alt":
		parse = parseTitledTextOrBlock
	case "table", "script", "text", "key":
		parse = parseTitledBlock
	case "choice", "multichoice", "matching", "ordering":
		parse = parseInstructionBlock
	case "fragment":
		parse = parseIDBlock
	case "variants":
		parse = parseBareBlock
	case "multifill":
		parse = parseMultifill
	case "media":
		parse = parseMedia
	case "resource-dir":
		parse = parseResourceDirs
	default:
		line := context.Line()
		line.LineType = model.LineTypeInvalid
		context.SetLine(line)
		addElement(context, model.ElementTypeUnparsed, start+1, next, nil)
		addSyntaxDiagnostic(context, diagnostics.P003, model.Location{
			Start: model.Position{Line: line.Number, Column: start + 1},
			End:   model.Position{Line: line.Number, Column: next},
		})
		return
	}
	line := context.Line()
	line.LineType = model.LineTypeTag
	context.SetLine(line)
	addElement(context, model.ElementTypeTag, start+1, next, &canonical)
	switch kind {
	case model.ElementTypeVersion, model.ElementTypeIdentifier, model.ElementTypeNumber:
		parseTokenTag(context, kind)
	case model.ElementTypeName, model.ElementTypeTitle, model.ElementTypeContent:
		parseFreeTag(context, kind)
	default:
		parse(context)
	}
}

// readTagName выделяет имя после неэкранированного начального @.
// nextColumn — колонка после имени; ok равен false при отсутствии имени.
// Краевые U+0020 и TAB перед @ пропускаются. Неэкранированные скобки
// и кавычка ограничивают имя; остальные символы не отбрасываются.
// При отсутствии @ nextColumn указывает на первый непробельный символ
// либо за конец строки; у одного @ — на колонку сразу после него.
func readTagName(context grammar.GrammarContext) (name string, nextColumn int, ok bool) {
	nextColumn = 1
	if context == nil {
		return
	}
	source := []rune(context.String())
	start := 0
	for start < len(source) && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	nextColumn = start + 1
	if start == len(source) || source[start] != '@' {
		return
	}
	start++
	end, escaped := start, false
	for end < len(source) {
		symbol := source[end]
		if symbol == ' ' || symbol == '\t' || (!escaped && (symbol == '{' || symbol == '}' || symbol == '"')) {
			break
		}
		escaped = symbol == '\\' && !escaped
		end++
	}
	return string(source[start:end]), end + 1, end > start
}
