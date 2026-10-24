// Package reporter печатает результат обработки каждого TXT и накапливает
// статистику запуска. Фактическое действие, число ошибок и причину сбоя он
// получает из storage.Report; model.Result нужен для числа строк и кода
// ранней фатальной диагностики, а также для подсчёта ошибок текущего разбора
// отдельно от IO001. Пакет не разбирает DSL и не записывает JSON.
//
// Форматы одной строки для успешно сохранённого файла:
// УСПЕХ: СОЗДАН | путь: "<абсолютный путь>" | строк: <N или нет данных> | ошибок: <N>
// УСПЕХ: ЗАМЕНЁН | путь: "<абсолютный путь>" | строк: <N или нет данных> | ошибок: <N>
//
// Формат одной строки для неудачной записи:
// ОШИБКА: <ДЕЙСТВИЕ> | путь: "<абсолютный путь>" | строк: <N или нет данных> | ошибок: <N> | причина: <текст ошибки>
// ДЕЙСТВИЕ берётся из storage.Report.Action: not-written — НЕ СОЗДАН,
// created — СОЗДАН, replaced — ЗАМЕНЁН. При not-written и nil в Err причиной
// служит «причина не указана». CR и LF выводятся как видимые \r и \n.
// Если модель содержит раннюю фатальную диагностику IO001/P001/P002/P013/P014
// последнего Processing, в конец независимо от статуса добавляется
// « | фатальная ошибка: <код>». Число строк в таком случае — «нет данных».
// Неизвестное число строк выводится как «нет данных» и при успешной записи.
// При пустом SourcePath выводится «путь: неизвестен»; известный путь
// заключается в кавычки с экранированием по правилам Go для %q.
//
// Формат единственной итоговой строки после всех Record:
// ИТОГО | всего: <N> | сохранено: <N> | сбоев: <N> | файлов с ошибками разбора: <N> | ошибок разбора: <N>
// Счётчики независимы: сохранение может сопровождаться ошибкой очистки,
// а ошибки разбора — техническим отказом записи. Фатальная ошибка DSL сама
// по себе не считается техническим сбоем. Если Record не вызывался, все числа
// равны нулю. Предупреждения и рекомендации в итог не входят.
package reporter

import (
	"fmt"

	"github.com/verdoga/dsl-parser/model"
	"github.com/verdoga/dsl-parser/storage"
)

// Reporter хранит счётчики одного последовательного запуска и печатает
// результаты в стандартную консоль; экземпляр принадлежит вызывающему коду.
type Reporter struct {
	// total — число полученных отчётов storage.
	total int
	// saved — число фактически созданных или заменённых JSON, включая сбой очистки.
	saved int
	// failed — число неуспешных исходов, включая фатальные диагностики, для exit code.
	failed int
	// technicalFailures — число файлов с техническим сбоем или отказом сохранения.
	technicalFailures int
	// filesWithParseErrors — число файлов с хотя бы одной ошибкой текущего разбора.
	filesWithParseErrors int
	// parseErrors — суммарное число ошибок текущего разбора без IO001.
	parseErrors int
}

// New создаёт Reporter с нулевыми счётчиками и выводом в стандартную консоль.
func New() *Reporter {
	return &Reporter{}
}

// Record принимает модель одного TXT и отчёт его записи, извлекает сводку,
// обновляет счётчики и печатает ровно одну строку в формате пакета.
// report.Err != nil либо report.Action == storage.ActionNotWritten означает
// «ОШИБКА»; при report.Err == nil и действии created/replaced — «УСПЕХ».
// Вызывается один раз после storage.Save для каждого переданного на обработку
// TXT. Дополнительную ошибку отдельным аргументом не принимает.
func (r *Reporter) Record(result model.Result, report storage.Report) {
	summary := summarizeFile(result, report)
	r.total++
	if isFailure(summary) {
		r.failed++
	}
	if summary.action == storage.ActionCreated || summary.action == storage.ActionReplaced {
		r.saved++
	}
	if summary.technicalFailure {
		r.technicalFailures++
	}
	if summary.parseErrors > 0 {
		r.filesWithParseErrors++
	}
	r.parseErrors += summary.parseErrors
	fmt.Println(formatFileLine(summary))
}

// Finish печатает одну итоговую строку и возвращает 0 при failed=0,
// иначе 1. Вызывается один раз после всех Record. Код 2 при ошибке
// параметров запуска выбирает вызывающий код до обработки файлов.
func (r *Reporter) Finish() int {
	fmt.Println(formatTotalLine(r.total, r.saved, r.technicalFailures, r.filesWithParseErrors, r.parseErrors))
	if r.failed != 0 {
		return 1
	}
	return 0
}
