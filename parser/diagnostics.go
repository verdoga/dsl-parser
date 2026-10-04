package parser

import (
	"fmt"
	"sort"
	"strings"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

// completeDiagnostic дополняет случай нормативными Severity и Scope, сохраняя
// конкретные Message, Location и RelatedLocations. При пустом Message берёт
// текст реестра. Для отдельной { сохраняет переданную грамматикой область
// element у P008; исходную строку проверяет importLineDiagnostics.
// Фатальность берётся только из реестра; значение из грамматики не используется.
func (p *Parser) completeDiagnostic(occurrence model.Diagnostic) (model.Diagnostic, error) {
	if p.diagnostics == nil {
		return model.Diagnostic{}, fmt.Errorf("не задан реестр диагностик")
	}
	description, found := p.diagnostics.Lookup(occurrence.DiagnosticCode)
	if !found || description == nil {
		return model.Diagnostic{}, fmt.Errorf("не зарегистрирована диагностика %q", occurrence.DiagnosticCode)
	}
	scope := description.Scope()
	if occurrence.DiagnosticCode == diagnostics.P008 && occurrence.DiagnosticScope == diagnostics.ScopeElement {
		scope = diagnostics.ScopeElement
	}
	occurrence.SeverityLevel = description.Severity()
	occurrence.DiagnosticScope = scope
	occurrence.Fatal = description.IsFatal()
	if occurrence.Message == "" {
		occurrence.Message = description.Message()
	}
	if occurrence.RelatedLocations == nil {
		occurrence.RelatedLocations = []model.Location{}
	}
	return occurrence, nil
}

// appendDiagnostic назначает не занятый ранее ID и Source текущего Processing,
// затем добавляет подготовленную диагностику в Result.Diagnostics.
func (p *Parser) appendDiagnostic(diagnostic model.Diagnostic) (id string) {
	for {
		id = fmt.Sprintf("d%d", p.nextDiagnosticID)
		p.nextDiagnosticID++
		occupied := false
		for _, previous := range p.result.Diagnostics {
			if previous.ID == id {
				occupied = true
				break
			}
		}
		if !occupied {
			break
		}
	}
	diagnostic.ID = id
	diagnostic.Source = p.processingID
	p.result.Diagnostics = append(p.result.Diagnostics, diagnostic)
	return id
}

// importLineDiagnostics переносит диагностики контекста без повторов кода на
// одном диапазоне в текущем запуске и привязывает element-диагностики к строке.
// Диагностики прежних запусков сохраняются независимо от совпадения диапазонов.
func (p *Parser) importLineDiagnostics(context grammar.GrammarContext, line *model.Line) error {
	if context == nil || line == nil {
		return fmt.Errorf("не задан контекст или строка для переноса диагностик")
	}
	for _, occurrence := range context.Diagnostics() {
		if occurrence.DiagnosticCode == diagnostics.P008 && strings.Trim(line.Raw, " \t") != "{" {
			occurrence.DiagnosticScope = diagnostics.ScopeLine
		}
		diagnostic, err := p.completeDiagnostic(occurrence)
		if err != nil {
			return err
		}
		id := ""
		for _, previous := range p.result.Diagnostics {
			sameRange := previous.Location == nil && diagnostic.Location == nil
			if previous.Location != nil && diagnostic.Location != nil {
				sameRange = *previous.Location == *diagnostic.Location
			}
			if previous.Source == p.processingID && previous.DiagnosticCode == diagnostic.DiagnosticCode && sameRange {
				diagnostic = previous
				id = previous.ID
				break
			}
		}
		if id == "" {
			diagnostic.ID = p.appendDiagnostic(diagnostic)
		}
		if diagnostic.DiagnosticScope == diagnostics.ScopeElement {
			if err := linkElementError(line, diagnostic); err != nil {
				return err
			}
		}
	}
	return nil
}

// linkElementError присоединяет ID element-диагностики к элементу с совпадающим
// диапазоном; ошибка означает нарушенную обратную ссылку. Повтор ID не добавляет.
func linkElementError(line *model.Line, diagnostic model.Diagnostic) error {
	if line == nil || diagnostic.ID == "" || diagnostic.DiagnosticScope != diagnostics.ScopeElement || diagnostic.Location == nil {
		return fmt.Errorf("диагностика %q: невозможно создать обратную ссылку элемента", diagnostic.ID)
	}
	location := diagnostic.Location
	if location.Start.Line == line.Number && location.End.Line == line.Number {
		for i := range line.Elements {
			element := &line.Elements[i]
			if element.Start != location.Start.Column || element.End != location.End.Column {
				continue
			}
			for _, id := range element.ErrorIDs {
				if id == diagnostic.ID {
					return nil
				}
			}
			element.ErrorIDs = append(element.ErrorIDs, diagnostic.ID)
			return nil
		}
	}
	return fmt.Errorf("строка %d: для диагностики %q нет элемента с совпадающим диапазоном", line.Number, diagnostic.ID)
}

// orderDiagnostics сортирует по Processing, началу Location и порядку обнаружения.
// Отсутствующая Location предшествует позициям исходника в том же запуске.
// ErrorIDs упорядочивает по результату, сохраняя сами ID и связи с ними.
func orderDiagnostics(result *model.Result) {
	sources := make(map[string]int, len(result.Processing))
	for i, processing := range result.Processing {
		sources[processing.ID] = i
	}
	sort.SliceStable(result.Diagnostics, func(i, j int) bool {
		left, right := result.Diagnostics[i], result.Diagnostics[j]
		if sources[left.Source] != sources[right.Source] {
			return sources[left.Source] < sources[right.Source]
		}
		if left.Location == nil || right.Location == nil {
			return left.Location == nil && right.Location != nil
		}
		if left.Location.Start.Line != right.Location.Start.Line {
			return left.Location.Start.Line < right.Location.Start.Line
		}
		return left.Location.Start.Column < right.Location.Start.Column
	})
	indexes := make(map[string]int, len(result.Diagnostics))
	for i, diagnostic := range result.Diagnostics {
		indexes[diagnostic.ID] = i
	}
	for i := range result.Lines {
		for j := range result.Lines[i].Elements {
			ids := result.Lines[i].Elements[j].ErrorIDs
			sort.SliceStable(ids, func(i, j int) bool { return indexes[ids[i]] < indexes[ids[j]] })
		}
	}
}

// setErrorFlags вычисляет Document.HasErrors и Line.HasErrors по Diagnostics.
// Учитывает начало основной позиции и ErrorIDs; дополнительные позиции,
// родители и дети строки не передают ей свои ошибки.
func setErrorFlags(result *model.Result) {
	result.Document.HasErrors = false
	errorLines := make(map[int]bool)
	errorIDs := make(map[string]bool)
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.SeverityLevel != diagnostics.SeverityError {
			continue
		}
		result.Document.HasErrors = true
		errorIDs[diagnostic.ID] = true
		if diagnostic.Location != nil {
			errorLines[diagnostic.Location.Start.Line] = true
		}
	}
	for i := range result.Lines {
		line := &result.Lines[i]
		line.HasErrors = errorLines[line.Number]
		for _, element := range line.Elements {
			for _, id := range element.ErrorIDs {
				if errorIDs[id] {
					line.HasErrors = true
				}
			}
		}
	}
}
