package gr12

import (
	"strings"

	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

// parseMedia выделяет TYPE и SOURCE; SOURCE может быть отдельным токеном
// либо путём в кавычках. Файлы и допустимость пути не проверяются.
// Элемент тега уже должен находиться в контексте.
func parseMedia(context grammar.GrammarContext) {
	if context == nil {
		return
	}
	column := 0
	for _, element := range context.Line().Elements {
		if element.ElementType == model.ElementTypeTag {
			column = element.End
			break
		}
	}
	if column == 0 {
		return
	}
	mediaType, next, present := readOperand(context, column, model.ElementTypeMediaType)
	if !checkRequiredOperand(context, present, next) {
		return
	}
	checkSeparator(context, column)
	canonical := strings.ToLower(mediaType.Raw)
	mediaType.Value = nil
	switch canonical {
	case "audio", "video", "image":
		mediaType.Value = &canonical
	}
	context.AddElement(mediaType)
	source := []rune(context.String())
	column = next
	for next <= len(source) && (source[next-1] == ' ' || source[next-1] == '\t') {
		next++
	}
	present = next <= len(source) && source[next-1] != '{' && source[next-1] != '}'
	if !checkRequiredOperand(context, present, next) {
		return
	}
	checkSeparator(context, column)
	operand, next, ok := readMediaSource(context, next)
	if !ok {
		return
	}
	context.AddElement(operand)
	column = next
	for next <= len(source) && (source[next-1] == ' ' || source[next-1] == '\t') {
		next++
	}
	if next <= len(source) && source[next-1] == '{' {
		checkSeparator(context, column)
		checkAllowedForm(context, "media", true)
		return
	}
	checkTagTail(context, next)
}

// parseResourceDirs выделяет пути, разделённые запятыми вне кавычек.
// Обратная косая черта в путях остаётся буквальным символом.
// Элемент тега уже должен находиться в контексте; ошибочные записи
// без согласованного кода диагностики пропускаются без выдуманных значений.
func parseResourceDirs(context grammar.GrammarContext) {
	if context == nil {
		return
	}
	column := 0
	for _, element := range context.Line().Elements {
		if element.ElementType == model.ElementTypeTag {
			column = element.End
			break
		}
	}
	source := []rune(context.String())
	if column < 1 || column > len(source)+1 {
		return
	}
	if strings.Trim(string(source[column-1:]), " \t") != "" {
		checkSeparator(context, column)
	}
	for column <= len(source) {
		element, next, ok := readResourcePath(context, column)
		if ok {
			context.AddElement(element)
		}
		if next <= column {
			return
		}
		column = next
	}
}

// readOperand выделяет отдельный операнд и точный диапазон исходного текста.
// nextColumn — колонка после него; ok равен false при отсутствии операнда.
// Пробелы и TAB перед операндом пропускаются. Неэкранированные скобки
// и кавычка, начинающая соседний операнд, в него не входят.
func readOperand(context grammar.GrammarContext, startColumn int, kind model.ElementType) (element model.Element, nextColumn int, ok bool) {
	nextColumn = startColumn
	if context == nil {
		return
	}
	source := []rune(context.String())
	if startColumn < 1 || startColumn > len(source)+1 {
		return
	}
	start := startColumn - 1
	for start < len(source) && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	end, escaped := start, false
	for end < len(source) {
		symbol := source[end]
		if symbol == ' ' || symbol == '\t' || (!escaped && (symbol == '{' || symbol == '}' || symbol == '"')) {
			break
		}
		if symbol == '\\' {
			escaped = !escaped
		} else {
			escaped = false
		}
		end++
	}
	nextColumn = end + 1
	if start == end {
		return
	}
	value := string(source[start:end])
	element = model.Element{ElementType: kind, Raw: value, Value: &value, Start: start + 1, End: end + 1, ErrorIDs: []string{}}
	return element, nextColumn, true
}

// readMediaSource находит конец SOURCE с учётом кавычек и их экранирования.
// nextColumn — колонка после SOURCE; ok равен false при незавершённой записи.
// При незакрытой кавычке nextColumn указывает за конец строки, элемент пуст.
func readMediaSource(context grammar.GrammarContext, startColumn int) (element model.Element, nextColumn int, ok bool) {
	nextColumn = startColumn
	if context == nil {
		return
	}
	source := []rune(context.String())
	if startColumn < 1 || startColumn > len(source)+1 {
		return
	}
	start := startColumn - 1
	for start < len(source) && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	if start == len(source) || source[start] != '"' {
		return readOperand(context, start+1, model.ElementTypeSource)
	}
	escaped := false
	for end := start + 1; end < len(source); end++ {
		symbol := source[end]
		if symbol == '"' && !escaped {
			value := string(source[start+1 : end])
			element = model.Element{ElementType: model.ElementTypeSource, Raw: string(source[start : end+1]), Value: &value, Start: start + 1, End: end + 2, ErrorIDs: []string{}}
			return element, end + 2, true
		}
		if symbol == '\\' {
			escaped = !escaped
		} else {
			escaped = false
		}
	}
	return element, len(source) + 1, false
}

// readResourcePath находит конец пути, учитывая кавычки и запятые.
// nextColumn — колонка следующего пути; ok равен false при незавершённой записи.
// Краевые пробелы и TAB вне кавычек исключаются; обратная косая черта буквальна.
// При пустой или некорректно ограниченной записи элемент не создаётся.
func readResourcePath(context grammar.GrammarContext, startColumn int) (element model.Element, nextColumn int, ok bool) {
	nextColumn = startColumn
	if context == nil {
		return
	}
	source := []rune(context.String())
	if startColumn < 1 || startColumn > len(source)+1 {
		return
	}
	start := startColumn - 1
	for start < len(source) && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	end, quotes, closing := start, 0, -1
	for end < len(source) {
		if source[end] == ',' && quotes%2 == 0 {
			break
		}
		if source[end] == '"' {
			quotes++
			closing = end
		}
		end++
	}
	nextColumn = end + 1
	if end < len(source) {
		nextColumn++
	}
	for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	if start == end || quotes%2 != 0 {
		return
	}
	value := string(source[start:end])
	if quotes != 0 {
		if quotes != 2 || source[start] != '"' || closing != end-1 {
			return
		}
		value = string(source[start+1 : end-1])
	}
	element = model.Element{ElementType: model.ElementTypeResourcePath, Raw: string(source[start:end]), Value: &value, Start: start + 1, End: end + 1, ErrorIDs: []string{}}
	return element, nextColumn, true
}
