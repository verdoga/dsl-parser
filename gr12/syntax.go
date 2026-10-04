package gr12

import (
	"strings"
	"unicode/utf8"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

// checkSeparator проверяет обязательный пробел между частями объявления.
// При отсутствии пробела или замене его табуляцией добавляет P004.
// column — колонка сразу после предшествующей части, начиная с 1.
func checkSeparator(context grammar.GrammarContext, column int) bool {
	if context == nil {
		return false
	}
	source := []rune(context.String())
	if column < 1 || column > len(source)+1 {
		return false
	}
	if column <= len(source) && source[column-1] == ' ' {
		return true
	}
	line := context.Line().Number
	addSyntaxDiagnostic(context, diagnostics.P004, model.Location{
		Start: model.Position{Line: line, Column: column},
		End:   model.Position{Line: line, Column: min(column+1, len(source)+1)},
	})
	return false
}

// checkRequiredOperand проверяет наличие обязательного отдельного операнда.
// При отсутствии добавляет P006; true означает, что операнд присутствует.
// column — колонка вставки отсутствующего операнда, начиная с 1.
func checkRequiredOperand(context grammar.GrammarContext, present bool, column int) bool {
	if present {
		return true
	}
	if context != nil && column >= 1 && column <= utf8.RuneCountInString(context.String())+1 {
		position := model.Position{Line: context.Line().Number, Column: column}
		addSyntaxDiagnostic(context, diagnostics.P006, model.Location{Start: position, End: position})
	}
	return false
}

// checkAllowedForm проверяет, поддерживает ли известный тег выбранную форму.
// При неподдерживаемой форме добавляет P005; true означает допустимую форму.
// tag — имя без @; неизвестное имя возвращает false без диагностики.
func checkAllowedForm(context grammar.GrammarContext, tag string, hasOpen bool) bool {
	allowed := false
	switch strings.ToLower(tag) {
	case "dsl-version", "document-id", "section", "order", "resource-dir", "header", "task",
		"endtask", "step", "speaking", "newpage", "media", "include", "question", "variant":
		allowed = !hasOpen
	case "table", "script", "text", "key", "choice", "multichoice", "matching", "ordering", "fragment", "variants":
		allowed = hasOpen
	case "editor", "instr", "hint", "answer", "example", "wordlist", "note", "alt", "multifill":
		allowed = true
	default:
		return false
	}
	if !allowed && context != nil {
		source := context.String()
		start := utf8.RuneCountInString(source) - utf8.RuneCountInString(strings.TrimLeft(source, " \t")) + 1
		end := utf8.RuneCountInString(strings.TrimRight(source, " \t")) + 1
		line := context.Line().Number
		addSyntaxDiagnostic(context, diagnostics.P005, model.Location{
			Start: model.Position{Line: line, Column: start},
			End:   model.Position{Line: line, Column: max(start, end)},
		})
	}
	return allowed
}

// checkTagTail проверяет остаток завершённой формы без свободного текста.
// При лишних токенах или содержимом добавляет P007.
func checkTagTail(context grammar.GrammarContext, startColumn int) {
	if context == nil {
		return
	}
	source := []rune(context.String())
	if startColumn < 1 || startColumn > len(source)+1 {
		return
	}
	start, end := startColumn-1, len(source)
	for start < end && (source[start] == ' ' || source[start] == '\t') {
		start++
	}
	for end > start && (source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	if start < end {
		line := context.Line().Number
		addSyntaxDiagnostic(context, diagnostics.P007, model.Location{
			Start: model.Position{Line: line, Column: start + 1},
			End:   model.Position{Line: line, Column: end + 1},
		})
	}
}

// checkOpeningTail проверяет текст после структурной открывающей скобки.
// При непустом хвосте добавляет P008; отдельное открытие также даёт P008.
func checkOpeningTail(context grammar.GrammarContext, braceColumn int) {
	if context == nil {
		return
	}
	source := []rune(context.String())
	if braceColumn < 1 || braceColumn > len(source) || source[braceColumn-1] != '{' {
		return
	}
	end := len(source)
	for end > braceColumn && (source[end-1] == ' ' || source[end-1] == '\t') {
		end--
	}
	if end > braceColumn || strings.Trim(string(source[:braceColumn-1]), " \t") == "" {
		line := context.Line().Number
		addSyntaxDiagnostic(context, diagnostics.P008, model.Location{
			Start: model.Position{Line: line, Column: braceColumn},
			End:   model.Position{Line: line, Column: end + 1},
		})
	}
}

// checkClosingTail проверяет текст рядом со структурной закрывающей скобкой.
// При непустом хвосте добавляет P010 и не создаёт вторую конструкцию строки.
func checkClosingTail(context grammar.GrammarContext, braceColumn int) {
	if context == nil {
		return
	}
	source := []rune(context.String())
	if braceColumn < 1 || braceColumn > len(source) || source[braceColumn-1] != '}' {
		return
	}
	trimmed := strings.Trim(string(source), " \t")
	if trimmed != "}" {
		start := len(source) - utf8.RuneCountInString(strings.TrimLeft(string(source), " \t")) + 1
		line := context.Line().Number
		addSyntaxDiagnostic(context, diagnostics.P010, model.Location{
			Start: model.Position{Line: line, Column: start},
			End:   model.Position{Line: line, Column: start + utf8.RuneCountInString(trimmed)},
		})
	}
}

// checkStrayBrace добавляет P012 для однозначно лишней неэкранированной
// скобки в объявлении вне структурной границы. Обычные текстовые кавычки
// не защищают скобки; специальные операнды разбираются отдельно.
// Диапазон соответствует проверяемому элементу, endColumn не включается.
// Для одного фрагмента добавляется одна диагностика с его полным диапазоном.
func checkStrayBrace(context grammar.GrammarContext, startColumn int, endColumn int) {
	checkTextBraces(context, startColumn, endColumn, false)
}

// checkTextBraces проверяет скобки одного готового текстового элемента.
// При allowAnswers парные скобки синтаксической формы _____{ANSWER} допустимы;
// допустимость её размещения проверяется валидатором, а не построчной грамматикой.
func checkTextBraces(context grammar.GrammarContext, startColumn, endColumn int, allowAnswers bool) {
	if context == nil {
		return
	}
	source := []rune(context.String())
	if startColumn < 1 || endColumn < startColumn || endColumn > len(source)+1 {
		return
	}
	if hasStrayBrace(source, startColumn-1, endColumn-1, allowAnswers) {
		line := context.Line().Number
		addSyntaxDiagnostic(context, diagnostics.P012, model.Location{
			Start: model.Position{Line: line, Column: startColumn},
			End:   model.Position{Line: line, Column: endColumn},
		})
	}
}

// addSyntaxDiagnostic записывает установленную для формы локальную ошибку.
// Идентификатор и source диагностики затем проставляет парсер.
// У отдельной { диагностика P008 имеет область element вместо line.
func addSyntaxDiagnostic(context grammar.GrammarContext, code diagnostics.Code, location model.Location) {
	if context == nil {
		return
	}
	description, found := context.Lookup(code)
	if !found || description == nil {
		return
	}
	scope := description.Scope()
	if code == diagnostics.P008 && strings.Trim(context.String(), " \t") == "{" {
		scope = diagnostics.ScopeElement
	}
	context.AddDiagnostic(model.Diagnostic{
		DiagnosticCode: code, SeverityLevel: description.Severity(), Message: description.Message(),
		DiagnosticScope: scope, Fatal: description.IsFatal(), Location: &location,
		RelatedLocations: []model.Location{},
	})
}

// addElement добавляет точный исходный фрагмент с колонками Unicode.
// endColumn не входит в диапазон; содержимое не интерпретируется повторно.
func addElement(context grammar.GrammarContext, kind model.ElementType, startColumn int, endColumn int, value *string) {
	if context == nil {
		return
	}
	source := []rune(context.String())
	if startColumn < 1 || endColumn < startColumn || endColumn > len(source)+1 {
		return
	}
	context.AddElement(model.Element{
		ElementType: kind, Raw: string(source[startColumn-1 : endColumn-1]), Value: value,
		Start: startColumn, End: endColumn, ErrorIDs: []string{},
	})
}
