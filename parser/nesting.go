package parser

import (
	"fmt"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

// assignParent выбирает родителя из активных логических родителей и блоков.
// Предварительный запрос выхода не меняет стеки. Заголовок остаётся корневым,
// а закрывающая строка ссылается на открытие блока, независимо от задания.
func (p *Parser) assignParent(line *model.Line) error {
	if line == nil {
		return fmt.Errorf("не задана строка для выбора родителя")
	}
	input := grammar.AssertionInput{Line: *line}
	parentID := 0
	if len(p.openBlocks) > 0 {
		parentID = p.openBlocks[len(p.openBlocks)-1]
		if parentID < 1 || parentID > len(p.result.Lines) || parentID >= line.Number || p.result.Lines[parentID-1].Number != parentID {
			return fmt.Errorf("строка %d: недоступно открытие блока %d", line.Number, parentID)
		}
		input.OpenBlock = &p.result.Lines[parentID-1]
	}
	if line.LineType != model.LineTypeBlockEnd && len(p.parents) > 0 {
		logicalID := p.parents[len(p.parents)-1]
		if logicalID < 1 || logicalID > len(p.result.Lines) || logicalID >= line.Number || p.result.Lines[logicalID-1].Number != logicalID {
			return fmt.Errorf("строка %d: недоступен логический родитель %d", line.Number, logicalID)
		}
		if logicalID > parentID {
			parentID = logicalID
			input.CandidateParent = &p.result.Lines[parentID-1]
			if p.leaveParentID == "" || p.assertions[p.leaveParentID] == nil {
				return fmt.Errorf("не задано утверждение выхода из родителя")
			}
			context := grammar.NewContext(line.Raw)
			context.SetAssertions([]grammar.Assertion{{ID: p.leaveParentID, Check: p.assertions[p.leaveParentID]}})
			if context.Assert(p.leaveParentID, input) {
				parentID = 0
				if input.OpenBlock != nil {
					parentID = input.OpenBlock.Number
				}
				if len(p.parents) > 1 && p.parents[len(p.parents)-2] > parentID {
					parentID = p.parents[len(p.parents)-2]
				}
			}
		}
	}
	line.ParentLine, line.NestingLevel = nil, 0
	if line.LineType == model.LineTypeHeading || parentID == 0 {
		return nil
	}
	if parentID < 1 || parentID > len(p.result.Lines) || parentID >= line.Number || p.result.Lines[parentID-1].Number != parentID {
		return fmt.Errorf("строка %d: недоступен выбранный родитель %d", line.Number, parentID)
	}
	line.ParentLine = &parentID
	line.NestingLevel = p.result.Lines[parentID-1].NestingLevel + 1
	return nil
}

// updateParents запрашивает обе структурные роли через context.Assert после
// сохранения строки. Утверждения не меняют стеки; true подтверждает переход.
// Фигурные блоки хранятся отдельно от логических родителей.
func (p *Parser) updateParents(context grammar.GrammarContext, line model.Line) error {
	if context == nil || line.Number < 1 || line.Number > len(p.result.Lines) || p.result.Lines[line.Number-1].Number != line.Number {
		return fmt.Errorf("строка %d: нет контекста или сохранённой строки для обновления родителей", line.Number)
	}
	if p.enterParentID == "" || p.leaveParentID == "" || p.assertions[p.enterParentID] == nil || p.assertions[p.leaveParentID] == nil {
		return fmt.Errorf("не заданы структурные утверждения")
	}
	input := grammar.AssertionInput{Line: line}
	parentID := 0
	if len(p.openBlocks) > 0 {
		parentID = p.openBlocks[len(p.openBlocks)-1]
		if parentID < 1 || parentID >= line.Number || p.result.Lines[parentID-1].Number != parentID {
			return fmt.Errorf("строка %d: недоступно открытие блока %d", line.Number, parentID)
		}
		input.OpenBlock = &p.result.Lines[parentID-1]
	}
	if len(p.parents) > 0 {
		logicalID := p.parents[len(p.parents)-1]
		if logicalID < 1 || logicalID >= line.Number || p.result.Lines[logicalID-1].Number != logicalID {
			return fmt.Errorf("строка %d: недоступен логический родитель %d", line.Number, logicalID)
		}
		if logicalID > parentID {
			parentID = logicalID
		}
	}
	if parentID != 0 {
		input.CandidateParent = &p.result.Lines[parentID-1]
	}
	leave := context.Assert(p.leaveParentID, input)
	enter := context.Assert(p.enterParentID, input)
	if line.LineType == model.LineTypeBlockEnd {
		if leave || input.OpenBlock == nil {
			return p.closeBlock(line)
		}
		return nil
	}
	if leave && len(p.parents) > 0 && p.parents[len(p.parents)-1] == parentID {
		p.parents = p.parents[:len(p.parents)-1]
	}
	if enter {
		if line.LineType == model.LineTypeBlockStart {
			p.openBlock(line)
		} else {
			p.parents = append(p.parents, line.Number)
		}
	}
	return nil
}

// openBlock добавляет номер строки открытия в стек фигурных блоков.
func (p *Parser) openBlock(line model.Line) {
	p.openBlocks = append(p.openBlocks, line.Number)
}

// closeBlock снимает открытый блок и логических родителей внутри него,
// сохраняя внешних родителей. При пустом стеке добавляет P009; строка с P010
// уже диагностирована как неправильное закрытие. Задание блоком не считается.
func (p *Parser) closeBlock(line model.Line) error {
	if len(p.openBlocks) > 0 {
		opening := p.openBlocks[len(p.openBlocks)-1]
		p.openBlocks = p.openBlocks[:len(p.openBlocks)-1]
		for len(p.parents) > 0 && p.parents[len(p.parents)-1] > opening {
			p.parents = p.parents[:len(p.parents)-1]
		}
		return nil
	}
	for _, diagnostic := range p.result.Diagnostics {
		if diagnostic.Source == p.processingID && (diagnostic.DiagnosticCode == diagnostics.P009 || diagnostic.DiagnosticCode == diagnostics.P010) && diagnostic.Location != nil && diagnostic.Location.Start.Line == line.Number {
			return nil
		}
	}
	for _, element := range line.Elements {
		if element.ElementType != model.ElementTypeBlockClose {
			continue
		}
		location := model.Location{Start: model.Position{Line: line.Number, Column: element.Start}, End: model.Position{Line: line.Number, Column: element.End}}
		diagnostic, err := p.completeDiagnostic(model.Diagnostic{DiagnosticCode: diagnostics.P009, Location: &location})
		if err != nil {
			return err
		}
		p.appendDiagnostic(diagnostic)
		return nil
	}
	return fmt.Errorf("строка %d: закрытие не содержит готового элемента block-close", line.Number)
}

// resetNesting удаляет ненадёжные открытые блоки и логических родителей,
// оставляя повреждённую строку корневой. Ранее сохранённые строки не меняет.
func (p *Parser) resetNesting(line *model.Line) {
	p.openBlocks, p.parents = nil, nil
	if line != nil {
		line.ParentLine, line.NestingLevel = nil, 0
	}
}

// checkOpenBlocks на EOF выдаёт фатальную P011 для каждого незакрытого блока.
// Сообщение содержит готовое имя тега и номер открытия, Location — диапазон
// открывающей скобки. Сохранённые строки и стеки не меняются.
func (p *Parser) checkOpenBlocks() error {
	for _, number := range p.openBlocks {
		if number < 1 || number > len(p.result.Lines) || p.result.Lines[number-1].Number != number {
			return fmt.Errorf("недоступна строка открытия блока %d", number)
		}
		line := p.result.Lines[number-1]
		tag := ""
		var location *model.Location
		for _, element := range line.Elements {
			switch element.ElementType {
			case model.ElementTypeTag:
				if element.Value != nil {
					tag = *element.Value
				}
			case model.ElementTypeBlockOpen:
				location = &model.Location{Start: model.Position{Line: number, Column: element.Start}, End: model.Position{Line: number, Column: element.End}}
			}
		}
		if line.LineType != model.LineTypeBlockStart || tag == "" || location == nil {
			return fmt.Errorf("строка %d: нет готового тега или открытия блока", number)
		}
		diagnostic, err := p.completeDiagnostic(model.Diagnostic{DiagnosticCode: diagnostics.P011, Message: fmt.Sprintf("Незакрытый блок @%s (строка %d)", tag, number), Location: location})
		if err != nil {
			return err
		}
		p.appendDiagnostic(diagnostic)
	}
	return nil
}
