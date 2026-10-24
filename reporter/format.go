package reporter

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/verdoga/dsl-parser/storage"
)

// actionText переводит фактическое storage.Action в «СОЗДАН», «ЗАМЕНЁН»
// или «НЕ СОЗДАН». Действие не угадывается по флагу Replace.
func actionText(action storage.Action) string {
	switch action {
	case storage.ActionCreated:
		return "СОЗДАН"
	case storage.ActionReplaced:
		return "ЗАМЕНЁН"
	case storage.ActionNotWritten:
		return "НЕ СОЗДАН"
	}
	return ""
}

// formatFileLine формирует русскоязычную строку одного файла по формату
// комментария к пакету; не обновляет счётчики и не печатает.
func formatFileLine(summary fileSummary) string {
	path := "неизвестен"
	if summary.path != "" {
		path = fmt.Sprintf("%q", summary.path)
	}
	lineCount := "нет данных"
	if summary.lineCount != nil {
		lineCount = strconv.Itoa(*summary.lineCount)
	}
	status := "УСПЕХ"
	failed := isFailure(summary)
	if failed {
		status = "ОШИБКА"
	}
	line := fmt.Sprintf("%s: %s | путь: %s | строк: %s | ошибок разбора: %d",
		status, actionText(summary.action), path, lineCount, summary.parseErrors)
	if failed {
		failure := "причина не указана"
		if summary.failure != nil {
			failure = summary.failure.Error()
		} else if summary.fatal != nil {
			failure = string(summary.fatal.DiagnosticCode) + ": " + summary.fatal.Message
		}
		failure = strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(failure)
		line += " | причина: " + failure
	}
	if summary.fatal != nil {
		line += " | фатальная ошибка: " + string(summary.fatal.DiagnosticCode)
	}
	return line
}

// formatTotalLine формирует итоговую строку по формату комментария к пакету;
// не обновляет счётчики и не печатает.
func formatTotalLine(total, saved, failures, filesWithParseErrors, parseErrors int) string {
	return fmt.Sprintf("ИТОГО | всего: %d | сохранено: %d | сбоев: %d | файлов с ошибками разбора: %d | ошибок разбора: %d",
		total, saved, failures, filesWithParseErrors, parseErrors)
}
