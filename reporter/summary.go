package reporter

import (
	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
	"github.com/verdoga/dsl-parser/storage"
)

// fileSummary хранит только сведения для вывода одного результата.
type fileSummary struct {
	// path — путь TXT из storage.Report.SourcePath или пустая строка.
	path string
	// lineCount — число физических строк; nil, если оно неизвестно.
	lineCount *int
	// parseErrors — единый счётчик для строки файла и итогов: нефатальные
	// ошибки текущего запуска, сохранённые в новом JSON. При отказе записи — 0.
	// Техническая IO001 и исторические диагностики не учитываются.
	parseErrors int
	// action — фактическое действие с JSON из storage.Report.Action.
	action storage.Action
	// failure — причина отказа или сбоя из storage.Report.Err.
	failure error
	// fatal — первая фатальная диагностика текущего запуска либо nil.
	fatal *model.Diagnostic
}

// currentDiagnostics выбирает диагностики последней записи Processing,
// чтобы не учитывать ошибки исторических запусков.
func currentDiagnostics(result model.Result) []model.Diagnostic {
	if len(result.Processing) == 0 {
		return nil
	}
	source := result.Processing[len(result.Processing)-1].ID
	var entries []model.Diagnostic
	for _, entry := range result.Diagnostics {
		if entry.Source == source {
			entries = append(entries, entry)
		}
	}
	return entries
}

// earlyFatal находит первую диагностику с Fatal=true и кодом IO001, P001,
// P002, P013 или P014. Иные коды не получают пометку раннего отказа.
// Возвращённая запись принадлежит переданной модели и не изменяется.
func earlyFatal(entries []model.Diagnostic) *model.Diagnostic {
	for i := range entries {
		if !entries[i].Fatal {
			continue
		}
		switch entries[i].DiagnosticCode {
		case diagnostics.IO001, diagnostics.P001, diagnostics.P002, diagnostics.P013, diagnostics.P014:
			return &entries[i]
		}
	}
	return nil
}

// summarizeFile переносит фактический итог записи из storage.Report и берёт
// число строк из model.Result. При ранней фатальной ошибке число строк
// оставляет неизвестным. Не определяет действие по console.Params.
func summarizeFile(result model.Result, report storage.Report) fileSummary {
	summary := fileSummary{
		path:      report.SourcePath,
		lineCount: result.Document.LineCount,
		action:    report.Action,
		failure:   report.Err,
	}
	entries := currentDiagnostics(result)
	if isSaved(summary) {
		for _, entry := range entries {
			if entry.SeverityLevel == diagnostics.SeverityError && !entry.Fatal && entry.DiagnosticCode != diagnostics.IO001 {
				summary.parseErrors++
			}
		}
	}
	if earlyFatal(entries) != nil {
		summary.lineCount = nil
	}
	if len(result.Processing) > 0 {
		currentID := result.Processing[len(result.Processing)-1].ID
		for i := range result.Diagnostics {
			if result.Diagnostics[i].Source == currentID && result.Diagnostics[i].Fatal {
				summary.fatal = &result.Diagnostics[i]
				break
			}
		}
	}
	return summary
}

// isSaved отражает факт установки JSON текущего запуска. Старый JSON после
// отказа замены не считается сохранённым результатом; ошибка очистки после
// установки нового JSON не отменяет сохранение.
func isSaved(summary fileSummary) bool {
	return summary.action == storage.ActionCreated || summary.action == storage.ActionReplaced
}

// isFailure сообщает об отказе обработки, записи или очистки. Фатальная
// диагностика сама означает сбой, даже если отчёт storage противоречит модели.
// Несколько причин в одном файле не увеличивают число сбоев повторно.
func isFailure(summary fileSummary) bool {
	return summary.fatal != nil || summary.failure != nil || summary.action == storage.ActionNotWritten
}
