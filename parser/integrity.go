package parser

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
)

// validateResult собирает проверки инвариантов перед возвратом модели.
// Ошибка здесь означает дефект конвейера, а не ещё одну DSL-диагностику.
func validateResult(result *model.Result) error {
	if err := checkLineSequence(result); err != nil {
		return err
	}
	if err := checkParentLinks(result.Lines); err != nil {
		return err
	}
	if err := checkDiagnosticSources(result); err != nil {
		return err
	}
	if err := checkDiagnosticRanges(result); err != nil {
		return err
	}
	if err := checkElementLinks(result); err != nil {
		return err
	}
	closingErrors := make(map[int]bool)
	for _, diagnostic := range result.Diagnostics {
		if (diagnostic.DiagnosticCode == diagnostics.P009 || diagnostic.DiagnosticCode == diagnostics.P010) &&
			diagnostic.SeverityLevel == diagnostics.SeverityError && diagnostic.DiagnosticScope == diagnostics.ScopeLine && diagnostic.Location != nil {
			closingErrors[diagnostic.Location.Start.Line] = true
		}
	}
	for _, line := range result.Lines {
		if line.LineType == model.LineTypeBlockEnd && line.ParentLine == nil && !closingErrors[line.Number] {
			return fmt.Errorf("строка %d: закрытие без открытого блока не имеет P009 или P010", line.Number)
		}
	}
	return nil
}

// checkLineSequence проверяет нумерацию, Raw/EOL и согласованность LineCount.
func checkLineSequence(result *model.Result) error {
	if result == nil {
		return fmt.Errorf("модель результата отсутствует")
	}
	if result.Document.LineCount == nil {
		if len(result.Lines) != 0 {
			return fmt.Errorf("при неизвестном LineCount набор строк должен быть пустым")
		}
		return nil
	}
	if *result.Document.LineCount != len(result.Lines) {
		return fmt.Errorf("LineCount %d не соответствует числу строк %d", *result.Document.LineCount, len(result.Lines))
	}
	for i, line := range result.Lines {
		if line.Number != i+1 {
			return fmt.Errorf("строка на позиции %d имеет номер %d", i+1, line.Number)
		}
		if !utf8.ValidString(line.Raw) || strings.ContainsAny(line.Raw, "\r\n") || (i == 0 && strings.HasPrefix(line.Raw, "\uFEFF")) {
			return fmt.Errorf("строка %d: Raw содержит недопустимый UTF-8, BOM или перевод строки", line.Number)
		}
		switch line.LineEnding {
		case model.LineEndingLF, model.LineEndingCRLF:
		case model.LineEndingNone:
			if i != len(result.Lines)-1 || line.Raw == "" {
				return fmt.Errorf("строка %d: отсутствует EOL до конца файла либо создана фиктивная пустая строка", line.Number)
			}
		default:
			return fmt.Errorf("строка %d: недопустимое окончание %q", line.Number, line.LineEnding)
		}
	}
	return nil
}

// checkParentLinks проверяет существование предшествующих родителей, глубину,
// корневые заголовки и связь закрывающей строки с открывающей.
func checkParentLinks(lines []model.Line) error {
	preceding := make(map[int]model.Line, len(lines))
	var openBlocks []int
	for _, line := range lines {
		if _, duplicate := preceding[line.Number]; line.Number < 1 || duplicate {
			return fmt.Errorf("недопустимый или повторный номер строки %d", line.Number)
		}
		if line.ParentLine == nil {
			if line.NestingLevel != 0 {
				return fmt.Errorf("строка %d: корневая глубина не равна нулю", line.Number)
			}
		} else {
			parent, found := preceding[*line.ParentLine]
			if !found || parent.Number >= line.Number || line.NestingLevel < 1 || line.NestingLevel-1 != parent.NestingLevel {
				return fmt.Errorf("строка %d: отсутствует предшествующий родитель или нарушена глубина", line.Number)
			}
			if line.LineType == model.LineTypeHeading {
				return fmt.Errorf("строка %d: заголовок имеет родителя", line.Number)
			}
		}
		switch line.LineType {
		case model.LineTypeBlockStart:
			openBlocks = append(openBlocks, line.Number)
		case model.LineTypeBlockEnd:
			if len(openBlocks) == 0 {
				if line.ParentLine != nil {
					return fmt.Errorf("строка %d: закрытие ссылается на неоткрытый блок", line.Number)
				}
			} else {
				if line.ParentLine == nil || *line.ParentLine != openBlocks[len(openBlocks)-1] {
					return fmt.Errorf("строка %d: закрытие не связано с последним открытым блоком", line.Number)
				}
				openBlocks = openBlocks[:len(openBlocks)-1]
			}
		}
		preceding[line.Number] = line
	}
	return nil
}

// checkDiagnosticSources проверяет ссылки Source на Processing.ID.
func checkDiagnosticSources(result *model.Result) error {
	if result == nil {
		return fmt.Errorf("модель результата отсутствует")
	}
	sources := make(map[string]bool, len(result.Processing))
	for _, processing := range result.Processing {
		if processing.ID == "" || sources[processing.ID] {
			return fmt.Errorf("пустой или повторный Processing.ID %q", processing.ID)
		}
		sources[processing.ID] = true
	}
	for _, diagnostic := range result.Diagnostics {
		if !sources[diagnostic.Source] {
			return fmt.Errorf("диагностика %q ссылается на неизвестный Processing.ID %q", diagnostic.ID, diagnostic.Source)
		}
	}
	return nil
}

// checkDiagnosticRanges проверяет Location и RelatedLocations по границам строк.
func checkDiagnosticRanges(result *model.Result) error {
	if result == nil {
		return fmt.Errorf("модель результата отсутствует")
	}
	columns := make(map[int]int, len(result.Lines))
	for _, line := range result.Lines {
		columns[line.Number] = utf8.RuneCountInString(line.Raw) + 1
	}
	for _, diagnostic := range result.Diagnostics {
		locations := make([]model.Location, 0, len(diagnostic.RelatedLocations)+1)
		if diagnostic.Location != nil {
			location := *diagnostic.Location
			if (diagnostic.DiagnosticScope == diagnostics.ScopeLine || diagnostic.DiagnosticScope == diagnostics.ScopeElement) && location.Start.Line != location.End.Line {
				return fmt.Errorf("диагностика %q: область %q выходит за одну строку", diagnostic.ID, diagnostic.DiagnosticScope)
			}
			locations = append(locations, location)
		} else if diagnostic.DiagnosticScope == diagnostics.ScopeElement {
			return fmt.Errorf("диагностика элемента %q не имеет Location", diagnostic.ID)
		}
		locations = append(locations, diagnostic.RelatedLocations...)
		seen := make(map[model.Location]bool, len(locations))
		for _, location := range locations {
			start, end := location.Start, location.End
			if start.Line < 1 || end.Line < start.Line || start.Column < 1 || end.Column < 1 ||
				start.Column > columns[start.Line] || end.Column > columns[end.Line] ||
				(start.Line == end.Line && end.Column < start.Column) {
				return fmt.Errorf("диагностика %q: диапазон %+v выходит за границы строк или обращён", diagnostic.ID, location)
			}
			if seen[location] {
				return fmt.Errorf("диагностика %q: повторный диапазон %+v", diagnostic.ID, location)
			}
			seen[location] = true
		}
	}
	return nil
}

// checkElementLinks проверяет ErrorIDs и обратные ссылки element-диагностик.
func checkElementLinks(result *model.Result) error {
	if result == nil {
		return fmt.Errorf("модель результата отсутствует")
	}
	indexes := make(map[string]int, len(result.Diagnostics))
	for i, diagnostic := range result.Diagnostics {
		if _, duplicate := indexes[diagnostic.ID]; diagnostic.ID == "" || duplicate {
			return fmt.Errorf("пустой или повторный Diagnostic.ID %q", diagnostic.ID)
		}
		indexes[diagnostic.ID] = i
	}
	linked := make(map[string]bool)
	for _, line := range result.Lines {
		hasUnparsed := false
		for _, element := range line.Elements {
			if element.ElementType == model.ElementTypeUnparsed {
				hasUnparsed = true
				if len(element.ErrorIDs) == 0 {
					return fmt.Errorf("строка %d: unparsed не имеет ссылки на диагностику", line.Number)
				}
			}
			lastIndex := -1
			for _, id := range element.ErrorIDs {
				index, found := indexes[id]
				if !found || index <= lastIndex {
					return fmt.Errorf("строка %d: неизвестный, повторный или неупорядоченный ErrorID %q", line.Number, id)
				}
				diagnostic := result.Diagnostics[index]
				location := model.Location{
					Start: model.Position{Line: line.Number, Column: element.Start},
					End:   model.Position{Line: line.Number, Column: element.End},
				}
				if diagnostic.DiagnosticScope != diagnostics.ScopeElement || diagnostic.Location == nil || *diagnostic.Location != location {
					return fmt.Errorf("строка %d: ErrorID %q не соответствует области и диапазону элемента", line.Number, id)
				}
				linked[id] = true
				lastIndex = index
			}
		}
		if line.LineType == model.LineTypeInvalid && !hasUnparsed {
			return fmt.Errorf("строка %d: invalid не содержит unparsed", line.Number)
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.DiagnosticScope == diagnostics.ScopeElement && !linked[diagnostic.ID] {
			return fmt.Errorf("диагностика элемента %q не имеет обратной ссылки", diagnostic.ID)
		}
	}
	return nil
}
