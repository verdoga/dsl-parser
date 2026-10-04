package gr12

import (
	"strconv"
	"strings"

	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

// classifyLine устанавливает тип пустой строки, заголовка, тега, содержимого,
// отдельной закрывающей скобки либо однозначно ошибочной конструкции.
// true означает установленный тип, false — невозможность его определить.
// Элементы и диагностики здесь не создаются; остальные поля Line сохраняются.
func classifyLine(context grammar.GrammarContext) bool {
	if context == nil {
		return false
	}
	source := []rune(strings.Trim(context.String(), " \t"))
	line := context.Line()
	line.LineType = model.LineTypeContent
	if len(source) == 0 {
		line.LineType = model.LineTypeBlank
	} else {
		switch source[0] {
		case '@':
			line.LineType = model.LineTypeTag
		case '#':
			level := 0
			for level < len(source) && source[level] == '#' {
				level++
			}
			line.LineType = model.LineTypeInvalid
			if level <= 3 && level < len(source) && source[level] == ' ' && strings.Trim(string(source[level:]), " \t") != "" {
				line.LineType = model.LineTypeHeading
			}
		case '}':
			line.LineType = model.LineTypeBlockEnd
		case '{':
			if len(source) == 1 {
				line.LineType = model.LineTypeInvalid
			}
		}
	}
	context.SetLine(line)
	return true
}

// parseLine направляет строку к разбору её частей без учёта других строк.
// true означает завершённый разбор, false — невозможность его закончить.
// У invalid без согласованной диагностики разбор остаётся частичным:
// исходник сохраняется, но искусственный unparsed без ошибки не создаётся.
// У media и resource-dir невыделенный операнд не считается завершённым
// только на основании того, что элемент тега уже добавлен.
func parseLine(context grammar.GrammarContext) bool {
	if context == nil {
		return false
	}
	line := context.Line()
	switch line.LineType {
	case model.LineTypeBlank:
		return true
	case model.LineTypeTag, model.LineTypeBlockStart:
		parseTag(context)
		// Проверяем полноту по уже выделенным частям, не запуская разбор
		// повторно и не добавляя диагностики для несогласованных ошибок.
		elements := context.Line().Elements[len(line.Elements):]
		if len(elements) == 0 || elements[0].ElementType != model.ElementTypeTag || elements[0].Value == nil {
			break
		}
		switch *elements[0].Value {
		case "media":
			if len(elements) == 2 && elements[1].ElementType == model.ElementTypeMediaType {
				source := []rune(context.String())
				tail := strings.TrimLeft(string(source[elements[1].End-1:]), " \t")
				// При начальной кавычке SOURCE отсутствует только тогда,
				// когда readMediaSource не нашёл её закрытие.
				if strings.HasPrefix(tail, "\"") {
					return false
				}
			}
		case "resource-dir":
			source := []rune(context.String())
			end := elements[0].End - 1
			for _, element := range elements[1:] {
				// Между выделенными путями могут остаться только
				// разделители. Иной фрагмент был пропущен читателем.
				if strings.Trim(string(source[end:element.Start-1]), " \t,") != "" {
					return false
				}
				end = element.End - 1
			}
			if strings.Trim(string(source[end:]), " \t,") != "" {
				return false
			}
		}
	case model.LineTypeHeading:
		parseHeading(context)
	case model.LineTypeContent:
		parseContent(context)
	case model.LineTypeBlockEnd:
		parseClosing(context)
	case model.LineTypeInvalid:
		source := []rune(context.String())
		if strings.Trim(string(source), " \t") != "{" {
			return false
		}
		start := 0
		for source[start] == ' ' || source[start] == '\t' {
			start++
		}
		addElement(context, model.ElementTypeUnparsed, start+1, start+2, nil)
		checkOpeningTail(context, start+1)
	default:
		return false
	}
	return len(context.Line().Elements) > len(line.Elements)
}

// parseHeading выделяет синтаксический уровень и текст заголовка.
// Поддерживаются #, ## и ### с U+0020 и непустым текстом; логическая
// вложенность не меняется. Неподдерживаемая запись остаётся без элементов.
func parseHeading(context grammar.GrammarContext) {
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
	markerEnd := start
	for markerEnd < end && source[markerEnd] == '#' {
		markerEnd++
	}
	level := markerEnd - start
	if level < 1 || level > 3 || markerEnd == end || source[markerEnd] != ' ' {
		return
	}
	titleStart := markerEnd
	for titleStart < end && (source[titleStart] == ' ' || source[titleStart] == '\t') {
		titleStart++
	}
	if titleStart == end {
		return
	}
	value := strconv.Itoa(level)
	addElement(context, model.ElementTypeHeadingLevel, start+1, markerEnd+1, &value)
	title := string(source[titleStart:end])
	addElement(context, model.ElementTypeTitle, titleStart+1, end+1, &title)
	checkStrayBrace(context, titleStart+1, end+1)
}

// parseClosing выделяет закрывающую скобку и вызывает проверку её хвоста.
// Наличие открытого блока эта функция не проверяет.
// Скобка должна быть первым символом после краевых U+0020 и TAB.
func parseClosing(context grammar.GrammarContext) {
	if context == nil {
		return
	}
	source := []rune(context.String())
	start := 0
	for start < len(source) && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	if start == len(source) || source[start] != '}' {
		return
	}
	value := "}"
	addElement(context, model.ElementTypeBlockClose, start+1, start+2, &value)
	checkClosingTail(context, start+1)
}
