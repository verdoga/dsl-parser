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
	// errors — число диагностик уровня error из storage.Report.ErrorCount.
	errors int
	// action — фактическое действие с JSON из storage.Report.Action.
	action storage.Action
	// failure — причина отказа или сбоя из storage.Report.Err.
	failure error
	// fatal — ранняя фатальная диагностика либо nil; модель не меняется.
	fatal *model.Diagnostic
}

// currentDiagnostics выбирает диагностики последней записи Processing,
// чтобы не искать раннюю фатальную ошибку среди исторических запусков.
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
		errors:    report.ErrorCount,
		action:    report.Action,
		failure:   report.Err,
	}
	if fatal := earlyFatal(currentDiagnostics(result)); fatal != nil {
		summary.lineCount = nil
		// Выборка содержит копии; сохраняем указатель на исходную запись модели.
		for i := range result.Diagnostics {
			if result.Diagnostics[i].ID == fatal.ID {
				summary.fatal = &result.Diagnostics[i]
				break
			}
		}
	}
	return summary
}

// isFailure сообщает, был ли отказ или сбой: true при ненулевой failure
// либо action=not-written; false при успешной установке JSON.
func isFailure(summary fileSummary) bool {
	return summary.failure != nil || summary.action == storage.ActionNotWritten
}
