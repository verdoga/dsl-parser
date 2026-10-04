package gr12

import (
	"strings"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

// parseBareTag разбирает endtask, speaking и newpage без операндов.
// Элемент тега уже должен находиться в контексте.
func parseBareTag(context grammar.GrammarContext) {
	if context == nil {
		return
	}
	line := context.Line()
	var tag model.Element
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeTag {
			tag = element
			break
		}
	}
	source := []rune(context.String())
	if tag.End < 1 || tag.End > len(source)+1 {
		return
	}
	line.LineType = model.LineTypeTag
	context.SetLine(line)
	start := tag.End - 1
	for start < len(source) && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	if start < len(source) && source[start] == '{' {
		checkSeparator(context, tag.End)
		checkAllowedForm(context, strings.TrimPrefix(strings.ToLower(tag.Raw), "@"), true)
		return
	}
	checkTagTail(context, tag.End)
}

// parseTokenTag разбирает отдельный обязательный операнд у dsl-version,
// document-id, order, task и include; kind задаёт тип элемента.
func parseTokenTag(context grammar.GrammarContext, kind model.ElementType) {
	if context == nil {
		return
	}
	line := context.Line()
	var tag model.Element
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeTag {
			tag = element
			break
		}
	}
	source := []rune(context.String())
	if tag.End < 1 || tag.End > len(source)+1 {
		return
	}
	line.LineType = model.LineTypeTag
	context.SetLine(line)
	operand, next, present := readOperand(context, tag.End, kind)
	checkRequiredOperand(context, present, next)
	if present {
		checkSeparator(context, tag.End)
		context.AddElement(operand)
	}
	start := next - 1
	for start < len(source) && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	if start < len(source) && source[start] == '{' {
		if present {
			checkSeparator(context, next)
		} else {
			checkSeparator(context, tag.End)
		}
		checkAllowedForm(context, strings.TrimPrefix(strings.ToLower(tag.Raw), "@"), true)
		return
	}
	checkTagTail(context, next)
}

// parseFreeTag разбирает свободное значение section, header, step, question
// и variant; kind различает name, title и content.
// Начальная либо конечная неэкранированная { обозначает попытку блочной
// формы. Остальные скобки остаются в значении и проверяются как текстовые.
func parseFreeTag(context grammar.GrammarContext, kind model.ElementType) {
	if context == nil {
		return
	}
	line := context.Line()
	var tag model.Element
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeTag {
			tag = element
			break
		}
	}
	source := []rune(context.String())
	if tag.End < 1 || tag.End > len(source)+1 {
		return
	}
	line.LineType = model.LineTypeTag
	context.SetLine(line)
	start, end := tag.End-1, len(source)
	for start < end && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	if start == end {
		return
	}
	checkSeparator(context, tag.End)
	brace, escaped := -1, false
	for i := start; i < end; i++ {
		if source[i] == '{' && !escaped && (i == start || i == end-1) {
			brace = i
			break
		}
		escaped = source[i] == '\\' && !escaped
	}
	if brace >= 0 {
		checkAllowedForm(context, strings.TrimPrefix(strings.ToLower(tag.Raw), "@"), true)
		end = brace
		for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
			end--
		}
	}
	if start < end {
		value := string(source[start:end])
		addElement(context, kind, start+1, end+1, &value)
		checkStrayBrace(context, start+1, end+1)
	}
}

// parsePlainTextOrBlock разбирает editor, instr, hint и answer:
// текст в объявлении либо открытие блока без названия.
// Начальная { открывает блок даже при ошибке хвоста. Конечная { после
// текста сохраняет открытие, но текст перед ней диагностируется как P007.
func parsePlainTextOrBlock(context grammar.GrammarContext) {
	if context == nil {
		return
	}
	line := context.Line()
	var tag model.Element
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeTag {
			tag = element
			break
		}
	}
	source := []rune(context.String())
	if tag.End < 1 || tag.End > len(source)+1 {
		return
	}
	line.LineType = model.LineTypeTag
	context.SetLine(line)
	start, end := tag.End-1, len(source)
	for start < end && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	if start == end {
		return
	}
	checkSeparator(context, tag.End)
	brace, escaped := -1, false
	for i := start; i < end; i++ {
		if source[i] == '{' && !escaped && (i == start || i == end-1) {
			brace = i
			break
		}
		escaped = source[i] == '\\' && !escaped
	}
	if brace < 0 {
		value := string(source[start:end])
		addElement(context, model.ElementTypeContent, start+1, end+1, &value)
		checkStrayBrace(context, start+1, end+1)
		return
	}
	if !checkAllowedForm(context, strings.TrimPrefix(strings.ToLower(tag.Raw), "@"), true) {
		return
	}
	if brace > start {
		end = brace
		for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
			end--
		}
		addSyntaxDiagnostic(context, diagnostics.P007, model.Location{
			Start: model.Position{Line: line.Number, Column: start + 1},
			End:   model.Position{Line: line.Number, Column: end + 1},
		})
		checkSeparator(context, end+1)
	}
	line.LineType = model.LineTypeBlockStart
	context.SetLine(line)
	value := "{"
	addElement(context, model.ElementTypeBlockOpen, brace+1, brace+2, &value)
	checkOpeningTail(context, brace+1)
}

// parseTitledTextOrBlock разбирает example, wordlist, note и alt:
// текст в объявлении либо открытие блока с необязательным названием.
// Первая неэкранированная { задаёт границу названия и блока.
func parseTitledTextOrBlock(context grammar.GrammarContext) {
	if context == nil {
		return
	}
	line := context.Line()
	var tag model.Element
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeTag {
			tag = element
			break
		}
	}
	source := []rune(context.String())
	if tag.End < 1 || tag.End > len(source)+1 {
		return
	}
	start, end := tag.End-1, len(source)
	for start < end && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	brace, escaped := -1, false
	for i := start; i < end; i++ {
		if source[i] == '{' && !escaped {
			brace = i
			break
		}
		escaped = source[i] == '\\' && !escaped
	}
	line.LineType = model.LineTypeTag
	kind := model.ElementTypeContent
	if brace >= 0 {
		line.LineType = model.LineTypeBlockStart
		kind = model.ElementTypeTitle
		end = brace
		for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
			end--
		}
	}
	context.SetLine(line)
	if start < len(source) {
		checkSeparator(context, tag.End)
	}
	if start < end {
		value := string(source[start:end])
		addElement(context, kind, start+1, end+1, &value)
		checkStrayBrace(context, start+1, end+1)
	}
	if brace >= 0 {
		if end > start {
			checkSeparator(context, end+1)
		}
		value := "{"
		addElement(context, model.ElementTypeBlockOpen, brace+1, brace+2, &value)
		checkOpeningTail(context, brace+1)
	}
}

// parseTitledBlock разбирает table, script, text и key с необязательным названием.
// При отсутствии открытия название сохраняется, форма получает P005.
func parseTitledBlock(context grammar.GrammarContext) {
	if context == nil {
		return
	}
	line := context.Line()
	var tag model.Element
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeTag {
			tag = element
			break
		}
	}
	source := []rune(context.String())
	if tag.End < 1 || tag.End > len(source)+1 {
		return
	}
	start, end := tag.End-1, len(source)
	for start < end && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	brace, escaped := -1, false
	for i := start; i < end; i++ {
		if source[i] == '{' && !escaped {
			brace = i
			break
		}
		escaped = source[i] == '\\' && !escaped
	}
	line.LineType = model.LineTypeTag
	if brace >= 0 {
		line.LineType = model.LineTypeBlockStart
		end = brace
		for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
			end--
		}
	}
	context.SetLine(line)
	checkAllowedForm(context, strings.TrimPrefix(strings.ToLower(tag.Raw), "@"), brace >= 0)
	if start < len(source) {
		checkSeparator(context, tag.End)
	}
	if start < end {
		value := string(source[start:end])
		addElement(context, model.ElementTypeTitle, start+1, end+1, &value)
		checkStrayBrace(context, start+1, end+1)
	}
	if brace >= 0 {
		if end > start {
			checkSeparator(context, end+1)
		}
		value := "{"
		addElement(context, model.ElementTypeBlockOpen, brace+1, brace+2, &value)
		checkOpeningTail(context, brace+1)
	}
}

// parseInstructionBlock разбирает choice, multichoice, matching и ordering
// с необязательной инструкцией перед открывающей скобкой.
// Инструкция сохраняется как content и при отсутствии обязательного открытия.
func parseInstructionBlock(context grammar.GrammarContext) {
	if context == nil {
		return
	}
	line := context.Line()
	var tag model.Element
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeTag {
			tag = element
			break
		}
	}
	source := []rune(context.String())
	if tag.End < 1 || tag.End > len(source)+1 {
		return
	}
	start, end := tag.End-1, len(source)
	for start < end && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	brace, escaped := -1, false
	for i := start; i < end; i++ {
		if source[i] == '{' && !escaped {
			brace = i
			break
		}
		escaped = source[i] == '\\' && !escaped
	}
	line.LineType = model.LineTypeTag
	if brace >= 0 {
		line.LineType = model.LineTypeBlockStart
		end = brace
		for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
			end--
		}
	}
	context.SetLine(line)
	checkAllowedForm(context, strings.TrimPrefix(strings.ToLower(tag.Raw), "@"), brace >= 0)
	if start < len(source) {
		checkSeparator(context, tag.End)
	}
	if start < end {
		value := string(source[start:end])
		addElement(context, model.ElementTypeContent, start+1, end+1, &value)
		checkStrayBrace(context, start+1, end+1)
	}
	if brace >= 0 {
		if end > start {
			checkSeparator(context, end+1)
		}
		value := "{"
		addElement(context, model.ElementTypeBlockOpen, brace+1, brace+2, &value)
		checkOpeningTail(context, brace+1)
	}
}

// parseIDBlock разбирает fragment с обязательным отдельным ID.
// Лишние слова между ID и открытием получают P007 и не становятся названием.
func parseIDBlock(context grammar.GrammarContext) {
	if context == nil {
		return
	}
	line := context.Line()
	var tag model.Element
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeTag {
			tag = element
			break
		}
	}
	source := []rune(context.String())
	if tag.End < 1 || tag.End > len(source)+1 {
		return
	}
	line.LineType = model.LineTypeTag
	context.SetLine(line)
	identifier, next, present := readOperand(context, tag.End, model.ElementTypeIdentifier)
	checkRequiredOperand(context, present, next)
	if present {
		checkSeparator(context, tag.End)
		context.AddElement(identifier)
	}
	start, end := next-1, len(source)
	for start < end && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	brace, escaped := -1, false
	for i := start; i < end; i++ {
		if source[i] == '{' && !escaped {
			brace = i
			break
		}
		escaped = source[i] == '\\' && !escaped
	}
	if !checkAllowedForm(context, strings.TrimPrefix(strings.ToLower(tag.Raw), "@"), brace >= 0) {
		checkTagTail(context, next)
		return
	}
	end = brace
	for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	if start < end {
		addSyntaxDiagnostic(context, diagnostics.P007, model.Location{
			Start: model.Position{Line: line.Number, Column: start + 1},
			End:   model.Position{Line: line.Number, Column: end + 1},
		})
		checkSeparator(context, end+1)
	} else if present {
		checkSeparator(context, next)
	} else {
		checkSeparator(context, tag.End)
	}
	line = context.Line()
	line.LineType = model.LineTypeBlockStart
	context.SetLine(line)
	value := "{"
	addElement(context, model.ElementTypeBlockOpen, brace+1, brace+2, &value)
	checkOpeningTail(context, brace+1)
}

// parseBareBlock разбирает variants без операндов.
// Лишний текст до открытия получает P007; открытие при этом сохраняется.
func parseBareBlock(context grammar.GrammarContext) {
	if context == nil {
		return
	}
	line := context.Line()
	var tag model.Element
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeTag {
			tag = element
			break
		}
	}
	source := []rune(context.String())
	if tag.End < 1 || tag.End > len(source)+1 {
		return
	}
	line.LineType = model.LineTypeTag
	context.SetLine(line)
	start, end := tag.End-1, len(source)
	for start < end && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	brace, escaped := -1, false
	for i := start; i < end; i++ {
		if source[i] == '{' && !escaped {
			brace = i
			break
		}
		escaped = source[i] == '\\' && !escaped
	}
	if !checkAllowedForm(context, strings.TrimPrefix(strings.ToLower(tag.Raw), "@"), brace >= 0) {
		checkTagTail(context, tag.End)
		return
	}
	checkSeparator(context, tag.End)
	end = brace
	for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	if start < end {
		addSyntaxDiagnostic(context, diagnostics.P007, model.Location{
			Start: model.Position{Line: line.Number, Column: start + 1},
			End:   model.Position{Line: line.Number, Column: end + 1},
		})
		checkSeparator(context, end+1)
	}
	line.LineType = model.LineTypeBlockStart
	context.SetLine(line)
	value := "{"
	addElement(context, model.ElementTypeBlockOpen, brace+1, brace+2, &value)
	checkOpeningTail(context, brace+1)
}

// parseMultifill разбирает четыре формы объявления multifill:
// с инструкцией или без неё, с открытием блока или без него.
// Первая неэкранированная { завершает инструкцию; хвост проверяется по P008.
func parseMultifill(context grammar.GrammarContext) {
	if context == nil {
		return
	}
	line := context.Line()
	var tag model.Element
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeTag {
			tag = element
			break
		}
	}
	source := []rune(context.String())
	if tag.End < 1 || tag.End > len(source)+1 {
		return
	}
	start, end := tag.End-1, len(source)
	for start < end && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	brace, escaped := -1, false
	for i := start; i < end; i++ {
		if source[i] == '{' && !escaped {
			brace = i
			break
		}
		escaped = source[i] == '\\' && !escaped
	}
	line.LineType = model.LineTypeTag
	if brace >= 0 {
		line.LineType = model.LineTypeBlockStart
		end = brace
		for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
			end--
		}
	}
	context.SetLine(line)
	if start < len(source) {
		checkSeparator(context, tag.End)
	}
	if start < end {
		value := string(source[start:end])
		addElement(context, model.ElementTypeContent, start+1, end+1, &value)
		checkStrayBrace(context, start+1, end+1)
	}
	if brace >= 0 {
		if end > start {
			checkSeparator(context, end+1)
		}
		value := "{"
		addElement(context, model.ElementTypeBlockOpen, brace+1, brace+2, &value)
		checkOpeningTail(context, brace+1)
	}
}
