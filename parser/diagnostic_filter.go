package parser

import (
	"fmt"
	"slices"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
)

// diagnosticRuleKey выбирает правила по точным версии DSL и коду диагностики.
type diagnosticRuleKey struct {
	Version string
	Code    diagnostics.Code
}

// diagnosticDecision отделяет отсутствие решения от окончательного ответа.
type diagnosticDecision uint8

const (
	diagnosticContinue diagnosticDecision = iota
	diagnosticKeep
	diagnosticSuppress
)

// diagnosticRuleInput содержит снимок данных, доступных правилу для чтения.
// Element отсутствует, если диапазон диагностики не совпадает с элементом.
// Parent — логический родитель, OpenBlock — ближайший открытый фигурный блок.
// Правило не получает Parser или GrammarContext и не меняет исходные данные.
type diagnosticRuleInput struct {
	Version    string
	Occurrence model.Diagnostic
	Line       model.Line
	Element    *model.Element
	Parent     *model.Line
	OpenBlock  *model.Line
}

// diagnosticRule принимает решение о переносе одной диагностики.
// Ошибка означает технический сбой правила, а не дополнительную DSL-диагностику.
type diagnosticRule func(diagnosticRuleInput) (diagnosticDecision, error)

// diagnosticRules хранит обработчики в порядке применения для каждого ключа.
// Набор принадлежит вызывающему коду; обход карты для выбора правил не нужен.
type diagnosticRules map[diagnosticRuleKey][]diagnosticRule

// builtinDiagnosticRules возвращает независимый набор встроенных правил.
func builtinDiagnosticRules() diagnosticRules {
	return diagnosticRules{
		{Version: "1.2", Code: diagnostics.P012}: {filterMultifillP012},
	}
}

// accept по умолчанию сохраняет диагностику. Каждое правило получает свою
// глубокую копию входа: его изменения не видны источнику и следующим правилам.
// Keep и Suppress завершают цепочку; ошибка также прекращает её выполнение.
func (rules diagnosticRules) accept(input diagnosticRuleInput) (bool, error) {
	key := diagnosticRuleKey{Version: input.Version, Code: input.Occurrence.DiagnosticCode}
	for i, rule := range rules[key] {
		if rule == nil {
			return false, fmt.Errorf("фильтр диагностик %q/%s: правило %d отсутствует", key.Version, key.Code, i+1)
		}
		decision, err := rule(cloneDiagnosticRuleInput(input))
		if err != nil {
			return false, fmt.Errorf("фильтр диагностик %q/%s: правило %d: %w", key.Version, key.Code, i+1, err)
		}
		switch decision {
		case diagnosticContinue:
			continue
		case diagnosticKeep:
			return true, nil
		case diagnosticSuppress:
			// P010 участвует в восстановлении стеков, P015 — в целостности
			// unparsed. Их удаление требует отдельной структурной обработки.
			if key.Code == diagnostics.P010 || key.Code == diagnostics.P015 {
				return false, fmt.Errorf("фильтр диагностик %q/%s: правило %d не может подавить структурную диагностику", key.Version, key.Code, i+1)
			}
			return false, nil
		default:
			return false, fmt.Errorf("фильтр диагностик %q/%s: правило %d вернуло неизвестное решение %d", key.Version, key.Code, i+1, decision)
		}
	}
	return true, nil
}

// newDiagnosticRuleInput отделяет вход от рабочей модели. Родителя и открытый
// блок выбирает вызывающий парсер; поиск элемента требует точного совпадения
// обеих позиций диагностики, включая номер строки и исключающую колонку End.
func newDiagnosticRuleInput(version string, occurrence model.Diagnostic, line model.Line, parent, openBlock *model.Line) diagnosticRuleInput {
	input := diagnosticRuleInput{
		Version: version, Occurrence: occurrence, Line: line,
		Parent: parent, OpenBlock: openBlock,
	}
	if location := occurrence.Location; location != nil && location.Start.Line == line.Number && location.End.Line == line.Number {
		for i := range line.Elements {
			element := &line.Elements[i]
			if element.Start == location.Start.Column && element.End == location.End.Column {
				input.Element = element
				break
			}
		}
	}
	return cloneDiagnosticRuleInput(input)
}

// cloneDiagnosticRuleInput копирует все изменяемые вложенные данные модели.
// Отдельно выбранный Element и совпадающие Parent/OpenBlock также независимы.
func cloneDiagnosticRuleInput(input diagnosticRuleInput) diagnosticRuleInput {
	if input.Occurrence.Location != nil {
		location := *input.Occurrence.Location
		input.Occurrence.Location = &location
	}
	input.Occurrence.RelatedLocations = slices.Clone(input.Occurrence.RelatedLocations)
	input.Line = cloneDiagnosticLine(input.Line)
	if input.Element != nil {
		element := cloneDiagnosticElement(*input.Element)
		input.Element = &element
	}
	if input.Parent != nil {
		parent := cloneDiagnosticLine(*input.Parent)
		input.Parent = &parent
	}
	if input.OpenBlock != nil {
		block := cloneDiagnosticLine(*input.OpenBlock)
		input.OpenBlock = &block
	}
	return input
}

func cloneDiagnosticLine(line model.Line) model.Line {
	if line.ParentLine != nil {
		parent := *line.ParentLine
		line.ParentLine = &parent
	}
	line.Elements = slices.Clone(line.Elements)
	for i, element := range line.Elements {
		line.Elements[i] = cloneDiagnosticElement(element)
	}
	return line
}

func cloneDiagnosticElement(element model.Element) model.Element {
	if element.Value != nil {
		value := *element.Value
		element.Value = &value
	}
	element.ErrorIDs = slices.Clone(element.ErrorIDs)
	return element
}
