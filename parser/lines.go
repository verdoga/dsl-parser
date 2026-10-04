package parser

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

// parseLines обходит строки по порядку, назначает связи и метаданные,
// переносит диагностики и сохраняет строку перед обновлением стеков.
// После фатальной структурной диагностики оставшиеся строки сохраняет
// в корневом лексическом режиме без восстановления ненадёжных связей.
func (p *Parser) parseLines(lines []model.Line) error {
	safe := false
	for _, source := range lines {
		if safe {
			source.LineType = model.LineTypeContent
		}
		context, err := p.parseLine(source)
		if err != nil {
			return err
		}
		line := context.Line()
		if err := p.assignParent(&line); err != nil {
			return err
		}
		if err := p.collectMetadata(&line); err != nil {
			return err
		}
		if err := checkElementPositions(line); err != nil {
			return err
		}
		firstDiagnostic := len(p.result.Diagnostics)
		if err := p.importLineDiagnostics(context, &line); err != nil {
			return err
		}
		for _, diagnostic := range p.result.Diagnostics[firstDiagnostic:] {
			if diagnostic.Fatal && (diagnostic.DiagnosticScope == diagnostics.ScopeBlock || diagnostic.DiagnosticScope == diagnostics.ScopeLine) {
				safe = true
				p.resetNesting(&line)
			}
		}
		if err := p.appendLine(source, line); err != nil {
			return err
		}
		if !safe {
			if err := p.updateParents(context, p.result.Lines[len(p.result.Lines)-1]); err != nil {
				return err
			}
		}
	}
	return nil
}

// parseLine создаёт контекст одной строки с диагностическим реестром и
// утверждениями выбранной версии. Обычный маршрут вызывает детекцию и
// оркестрацию; текстовый — зарегистрированный разбор заданного типа.
// Состояние блоков остаётся в PARSER. Незавершённые фрагменты получают P015.
func (p *Parser) parseLine(source model.Line) (grammar.GrammarContext, error) {
	context := grammar.NewContext(source.Raw)
	context.SetDiagnosticRegistry(p.diagnostics)
	assertions := make([]grammar.Assertion, 0, len(p.assertions))
	for id, check := range p.assertions {
		assertions = append(assertions, grammar.Assertion{ID: id, Check: check})
	}
	context.SetAssertions(assertions)
	context.SetLine(source)
	textMode := source.LineType == model.LineTypeContent
	if !textMode && len(p.openBlocks) > 0 {
		number := p.openBlocks[len(p.openBlocks)-1]
		if number < 1 || number > len(p.result.Lines) {
			return nil, fmt.Errorf("строка %d: отсутствует открытый блок %d", source.Number, number)
		}
		for _, element := range p.result.Lines[number-1].Elements {
			if element.ElementType != model.ElementTypeTag || element.Value == nil {
				continue
			}
			switch *element.Value {
			case "editor", "example", "wordlist", "table", "script", "text", "key", "note", "alt", "hint", "answer", "multifill":
				textMode = true
			}
		}
		if strings.HasPrefix(strings.TrimLeft(source.Raw, " \t"), "}") {
			textMode = false
		}
	}
	complete := true
	if textMode {
		selected, found := p.grammars.Lookup(p.version)
		if !found || len(selected.LineParserFuncs()) == 0 {
			return nil, fmt.Errorf("версия %q: не задан построчный разбор содержимого", p.version)
		}
		line := source
		line.LineType = model.LineTypeContent
		if strings.Trim(source.Raw, " \t") == "" {
			line.LineType = model.LineTypeBlank
		}
		context.SetLine(line)
		for _, parse := range selected.LineParserFuncs() {
			if parse == nil {
				return nil, fmt.Errorf("версия %q: пустая функция разбора содержимого", p.version)
			}
			if !parse(context) {
				complete = false
				break
			}
		}
	} else {
		if len(p.orchestrators) == 0 {
			return nil, fmt.Errorf("версия %q: не задана оркестрация", p.version)
		}
		for _, detect := range p.detections {
			if detect == nil {
				return nil, fmt.Errorf("версия %q: пустая функция детекции", p.version)
			}
			if !detect(context) {
				complete = false
				break
			}
		}
		if complete {
			for _, orchestrate := range p.orchestrators {
				if orchestrate == nil {
					return nil, fmt.Errorf("версия %q: пустая функция оркестрации", p.version)
				}
				if !orchestrate(context) {
					complete = false
					break
				}
			}
		}
	}
	line := context.Line()
	line.Number, line.Raw, line.LineEnding = source.Number, source.Raw, source.LineEnding
	if err := checkElementPositions(line); err != nil {
		return nil, err
	}
	if !complete {
		if len(line.Elements) == 0 || line.LineType == "" {
			line.LineType = model.LineTypeInvalid
		}
		runes := []rune(source.Raw)
		elements := make([]model.Element, 0, len(line.Elements)+1)
		start, added := 1, false
		for i := 0; i <= len(line.Elements); i++ {
			end := len(runes) + 1
			if i < len(line.Elements) {
				end = line.Elements[i].Start
			}
			for start < end && (runes[start-1] == ' ' || runes[start-1] == '\t') {
				start++
			}
			for end > start && (runes[end-2] == ' ' || runes[end-2] == '\t') {
				end--
			}
			if start < end {
				elements = append(elements, model.Element{ElementType: model.ElementTypeUnparsed, Raw: string(runes[start-1 : end-1]), Start: start, End: end})
				context.AddDiagnostic(model.Diagnostic{DiagnosticCode: diagnostics.P015, Location: &model.Location{Start: model.Position{Line: source.Number, Column: start}, End: model.Position{Line: source.Number, Column: end}}})
				added = true
			}
			if i < len(line.Elements) {
				elements = append(elements, line.Elements[i])
				start = line.Elements[i].End
			}
		}
		if !added {
			end := len(runes) + 1
			elements = append(elements, model.Element{ElementType: model.ElementTypeUnparsed, Start: end, End: end})
			context.AddDiagnostic(model.Diagnostic{DiagnosticCode: diagnostics.P015, Location: &model.Location{Start: model.Position{Line: source.Number, Column: end}, End: model.Position{Line: source.Number, Column: end}}})
		}
		line.Elements = elements
	}
	context.SetLine(line)
	return context, nil
}

// appendLine сохраняет готовую строку с исходными Number, Raw и EOL.
// Elements и ErrorIDs копируются и представляются пустыми массивами вместо nil.
func (p *Parser) appendLine(source model.Line, parsed model.Line) error {
	if source.Number != len(p.result.Lines)+1 {
		return fmt.Errorf("нарушен порядок сохранения строки %d", source.Number)
	}
	parsed.Number, parsed.Raw, parsed.LineEnding = source.Number, source.Raw, source.LineEnding
	parsed.Elements = append([]model.Element{}, parsed.Elements...)
	for i := range parsed.Elements {
		parsed.Elements[i].ErrorIDs = append([]string{}, parsed.Elements[i].ErrorIDs...)
	}
	p.result.Lines = append(p.result.Lines, parsed)
	return nil
}

// checkElementPositions проверяет диапазоны в Unicode-колонках с исключающим
// End, точные Raw, порядок и отсутствие пересечений. Пустые вставки допустимы.
func checkElementPositions(line model.Line) error {
	if !utf8.ValidString(line.Raw) {
		return fmt.Errorf("строка %d: Raw не является UTF-8", line.Number)
	}
	runes := []rune(line.Raw)
	end := 1
	for _, element := range line.Elements {
		if element.Start < end || element.End < element.Start || element.End > len(runes)+1 {
			return fmt.Errorf("строка %d: нарушен диапазон элемента [%d,%d)", line.Number, element.Start, element.End)
		}
		if !slices.Equal([]rune(element.Raw), runes[element.Start-1:element.End-1]) || !utf8.ValidString(element.Raw) {
			return fmt.Errorf("строка %d: Raw элемента не соответствует диапазону", line.Number)
		}
		end = element.End
	}
	return nil
}
