package gr12

import (
	"strings"

	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

// IncreasesNesting — идентификатор вопроса, открывает ли строка новый
// уровень логической вложенности DSL v1.2.
const IncreasesNesting grammar.AssertionID = "increases-nesting"

// DecreasesNesting — идентификатор вопроса, завершает ли строка текущий
// уровень логической вложенности DSL v1.2.
const DecreasesNesting grammar.AssertionID = "decreases-nesting"

// increasesNesting подтверждает структурное открытие фигурного блока
// или однострочный тег task. Не меняет Line и состояние парсера.
// true означает открытие уровня, false — отсутствие открытия.
// Локальные ошибки не отменяют открытие; попытка блочной формы task
// исключается по границе после выделенного ID либо имени тега без ID.
func increasesNesting(input grammar.AssertionInput) bool {
	if input.Line.LineType != model.LineTypeTag && input.Line.LineType != model.LineTypeBlockStart {
		return false
	}
	name, tagEnd, operandEnd, hasOpen := "", 0, 0, false
	for _, element := range input.Line.Elements {
		switch element.ElementType {
		case model.ElementTypeTag:
			if element.Value != nil {
				name, tagEnd = *element.Value, element.End
			}
		case model.ElementTypeIdentifier:
			operandEnd = element.End
		case model.ElementTypeBlockOpen:
			hasOpen = element.Raw == "{"
		}
	}
	if input.Line.LineType == model.LineTypeBlockStart {
		return hasOpen && checkAllowedForm(nil, name, true)
	}
	if name != "task" || hasOpen {
		return false
	}
	if operandEnd == 0 {
		operandEnd = tagEnd
	}
	source := []rune(input.Line.Raw)
	if operandEnd >= 1 && operandEnd <= len(source)+1 {
		tail := strings.TrimLeft(string(source[operandEnd-1:]), " \t")
		if strings.HasPrefix(tail, "{") {
			return false
		}
	}
	return true
}

// decreasesNesting подтверждает отдельное закрытие открытого блока либо
// завершение task на уровне CandidateParent по правилам DSL v1.2.
// true означает закрытие текущего уровня, false — отсутствие закрытия.
// Структурная } при наличии OpenBlock подтверждает закрытие независимо
// от CandidateParent и ошибки хвоста. Выбор закрываемого блока делает парсер;
// это подтверждение само по себе не завершает task вокруг дочернего блока.
func decreasesNesting(input grammar.AssertionInput) bool {
	if input.Line.LineType == model.LineTypeBlockEnd {
		if input.OpenBlock == nil {
			return false
		}
		for _, element := range input.Line.Elements {
			if element.ElementType == model.ElementTypeBlockClose && element.Raw == "}" {
				return true
			}
		}
		return false
	}
	parent := input.CandidateParent
	if parent == nil || parent.LineType != model.LineTypeTag {
		return false
	}
	parentIsTask := false
	for _, element := range parent.Elements {
		if element.ElementType == model.ElementTypeTag && element.Value != nil && *element.Value == "task" {
			parentIsTask = true
			break
		}
	}
	if !parentIsTask {
		return false
	}
	if input.Line.LineType == model.LineTypeHeading {
		for _, element := range input.Line.Elements {
			if element.ElementType == model.ElementTypeHeadingLevel && element.Value != nil && *element.Value == "3" {
				return true
			}
		}
		return false
	}
	if input.Line.LineType != model.LineTypeTag && input.Line.LineType != model.LineTypeBlockStart {
		return false
	}
	for _, element := range input.Line.Elements {
		if element.ElementType != model.ElementTypeTag || element.Value == nil {
			continue
		}
		switch *element.Value {
		case "task", "endtask", "header", "newpage", "variants":
			return true
		case "variant":
			if input.OpenBlock == nil || input.OpenBlock.LineType != model.LineTypeBlockStart || parent.ParentLine == nil || *parent.ParentLine != input.OpenBlock.Number {
				return false
			}
			for _, opening := range input.OpenBlock.Elements {
				if opening.ElementType == model.ElementTypeTag && opening.Value != nil && *opening.Value == "variants" {
					return true
				}
			}
			return false
		}
	}
	return false
}
