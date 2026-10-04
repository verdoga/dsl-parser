package parser

import (
	"fmt"

	"github.com/verdoga/dsl-parser/model"
)

// collectMetadata читает только готовые элементы строк tag и heading.
// Raw содержимого повторно как DSL не распознаёт; первоначальная версия
// уже отмечена при подготовке входа. Повтор одиночного поля не извлекается.
func (p *Parser) collectMetadata(line *model.Line) error {
	if line == nil {
		return fmt.Errorf("не задана строка для сбора метаданных")
	}
	if line.LineType != model.LineTypeTag && line.LineType != model.LineTypeHeading {
		return nil
	}
	repeated, err := p.checkMetadataRepeat(line)
	if err != nil || repeated {
		return err
	}
	if line.LineType == model.LineTypeHeading {
		p.extractHeadingValue(*line)
		return nil
	}
	return extractMetadataValue(&p.result.Document.Metadata, *line)
}

// checkMetadataRepeat отмечает первое объявление по готовому имени или уровню.
// true означает повтор одиночного поля: его значение сбрасывается в nil.
// Первоначальная строка версии и повторяемые пути ресурсов не дают true.
// Строки сохраняют распознанный тип; P003 и новые коды не создаются.
func (p *Parser) checkMetadataRepeat(line *model.Line) (repeated bool, err error) {
	if line == nil {
		return false, fmt.Errorf("не задана строка для проверки повтора метаданных")
	}
	var kind model.ElementType
	switch line.LineType {
	case model.LineTypeTag:
		kind = model.ElementTypeTag
	case model.LineTypeHeading:
		kind = model.ElementTypeHeadingLevel
	default:
		return false, nil
	}
	var marker *string
	count := 0
	for _, element := range line.Elements {
		if element.ElementType == kind {
			marker = element.Value
			count++
		}
	}
	if count != 1 || marker == nil {
		return false, nil
	}
	key := ""
	var field **string
	metadata := &p.result.Document.Metadata
	if kind == model.ElementTypeTag {
		switch *marker {
		case "dsl-version":
			field = &p.result.Document.DSLVersion
		case "document-id":
			field = &metadata.DocumentID
		case "section":
			field = &metadata.Section
		case "order":
			field = &metadata.Order
		case "resource-dir":
		default:
			return false, nil
		}
		key = "@" + *marker
	} else {
		switch *marker {
		case "1":
			key, field = "#", &metadata.Title
		case "2":
			key, field = "##", &metadata.Subtitle
		case "3":
			key = "###"
		default:
			return false, nil
		}
	}
	first, seen := p.seenMetadata[key]
	if !seen {
		p.seenMetadata[key] = line.Number
		return false, nil
	}
	if field == nil || (key == "@dsl-version" && first == 1 && line.Number == 1) {
		return false, nil
	}
	*field = nil
	return true, nil
}

// extractMetadataValue переносит готовые значения в Document.Metadata.
// Одиночное поле требует ровно одного значения и отсутствия unparsed.
// Пути извлекаются по отдельности в порядке элементов, включая одинаковые.
// Для повторённого одиночного поля функция не вызывается.
func extractMetadataValue(metadata *model.DocumentMetadata, line model.Line) error {
	if metadata == nil {
		return fmt.Errorf("не задана модель метаданных")
	}
	if line.LineType != model.LineTypeTag {
		return nil
	}
	var tag *string
	tags := 0
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeTag {
			tag = element.Value
			tags++
		}
	}
	if tags != 1 || tag == nil {
		return nil
	}
	var kind model.ElementType
	var field **string
	switch *tag {
	case "document-id":
		kind, field = model.ElementTypeIdentifier, &metadata.DocumentID
	case "section":
		kind, field = model.ElementTypeName, &metadata.Section
	case "order":
		kind, field = model.ElementTypeNumber, &metadata.Order
	case "resource-dir":
		for _, element := range line.Elements {
			if element.ElementType == model.ElementTypeResourcePath && element.Value != nil {
				metadata.ResourceDirs = append(metadata.ResourceDirs, *element.Value)
			}
		}
		return nil
	default:
		return nil
	}
	*field = nil
	var value *string
	count, ambiguous := 0, false
	for _, element := range line.Elements {
		if element.ElementType == model.ElementTypeUnparsed {
			ambiguous = true
		}
		if element.ElementType == kind {
			value = element.Value
			count++
		}
	}
	if count == 1 && value != nil && !ambiguous {
		copied := *value
		*field = &copied
	}
	return nil
}

// extractHeadingValue заполняет Title или Subtitle по готовому уровню и title.
// Неоднозначный текст оставляет nil; третий уровень не является метаданными.
func (p *Parser) extractHeadingValue(line model.Line) {
	if line.LineType != model.LineTypeHeading {
		return
	}
	var level, title *string
	levels, titles, ambiguous := 0, 0, false
	for _, element := range line.Elements {
		switch element.ElementType {
		case model.ElementTypeHeadingLevel:
			level = element.Value
			levels++
		case model.ElementTypeTitle:
			title = element.Value
			titles++
		case model.ElementTypeUnparsed:
			ambiguous = true
		}
	}
	if levels != 1 || level == nil {
		return
	}
	var field **string
	switch *level {
	case "1":
		field = &p.result.Document.Metadata.Title
	case "2":
		field = &p.result.Document.Metadata.Subtitle
	default:
		return
	}
	*field = nil
	if titles == 1 && title != nil && !ambiguous {
		copied := *title
		*field = &copied
	}
}
