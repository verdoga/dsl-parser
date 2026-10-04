package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/verdoga/dsl-parser/console"
	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
	"github.com/verdoga/dsl-parser/parser"
	"github.com/verdoga/dsl-parser/reporter"
	"github.com/verdoga/dsl-parser/storage"
)

// runner хранит готовые зависимости одного последовательного запуска.
// Модель, поток чтения и время отдельного файла создаются внутри processFile.
type runner struct {
	// params — проверенные параметры запуска, включая разрешение замены.
	params console.Params

	// version — непустая версия исполняемого инструмента для Processing.Version.
	version string

	// grammars — общий реестр грамматик, не изменяемый во время обработки.
	grammars grammar.Registry

	// diagnostics — результат diagnostics.NewRegistry, общий для всех файлов.
	diagnostics diagnostics.Registry

	// assertions — функции проверок по версии DSL и публичному ID грамматики.
	assertions map[string]map[grammar.AssertionID]grammar.AssertionFunc

	// reporter — накопитель консольного результата одного запуска.
	reporter *reporter.Reporter
}

// processFile фиксирует startedAt перед попыткой os.Open и создаёт newResult.
// При ошибке открытия передаёт parser.New читатель failedReader: parser.readBytes
// создаст IO001 в модели. Иначе передаёт открытый файл в parser.New и закрывает
// его после Parse. Устанавливает DurationMs после разбора и закрытия, до записи.
// При штатном возврате Parse, включая результат с фатальной диагностикой,
// вызывает storage.Save и передаёт модель с его Report в reporter.Record.
// При технической ошибке Parse либо закрытия не вызывает storage.Save:
// recordFailure передаёт reporter отчёт без записи. Ошибки разбора и закрытия
// получают контекст операций и объединяются через errors.Join.
// Для каждого пути Record вызывается ровно один раз; следующий файл
// обрабатывается независимо.
func (r *runner) processFile(path string) {
	startedAt := time.Now()
	result := r.newResult(path, startedAt)
	file, openErr := os.Open(path)
	var reader io.Reader = file
	if openErr != nil {
		reader = failedReader{err: openErr}
	}
	parseErr := parser.New(reader, &result, r.grammars, r.diagnostics, r.assertions).Parse()
	if parseErr != nil {
		parseErr = fmt.Errorf("не удалось разобрать файл %q: %w", path, parseErr)
	}
	var closeErr error
	if file != nil {
		if err := file.Close(); err != nil {
			closeErr = fmt.Errorf("не удалось закрыть файл %q: %w", path, err)
		}
	}
	finishProcessing(&result, startedAt)
	if err := errors.Join(parseErr, closeErr); err != nil {
		r.recordFailure(result, path, err)
		return
	}
	report := storage.Save(&result, r.params)
	r.reporter.Record(result, report)
}

// newResult предзаполняет модель согласно требованиям parser.prepareResult:
// Format=model.FormatVersion1; Document.FilePath — копия абсолютного пути
// discovery; Document.FileName — filepath.Base этого пути; HasErrors=false.
// Неизвестные значения Document, включая DSLVersion, LineCount, Encoding,
// ByteLength и SHA256, остаются nil; Metadata.ResourceDirs — пустой срез.
// Processing содержит ровно одну запись с ID=processingID, Tool=toolName,
// непустой Version из r.version, StartedAt в UTC по RFC 3339 с Z и пока nil
// DurationMs. Lines и Diagnostics — пустые срезы, без прежних ID и записей.
// Дальнейшие байтовые сведения, строки, диагностики и признаки заполняет parser.
func (r *runner) newResult(path string, startedAt time.Time) model.Result {
	name := filepath.Base(path)
	version := r.version
	started := startedAt.UTC().Format(time.RFC3339)
	return model.Result{
		Format: model.FormatVersion1,
		Document: model.Document{
			FilePath: &path,
			FileName: &name,
			Metadata: model.DocumentMetadata{ResourceDirs: []string{}},
		},
		Processing: []model.Processing{{
			ID: processingID, Tool: toolName, Version: &version, StartedAt: &started,
		}},
		Lines:       []model.Line{},
		Diagnostics: []model.Diagnostic{},
	}
}

// finishProcessing устанавливает неотрицательное DurationMs в миллисекундах
// у единственной записи Processing непосредственно перед storage.Save либо
// отчётом об отказе; время сохранения JSON в длительность не входит.
func finishProcessing(result *model.Result, startedAt time.Time) {
	duration := max(0, int(time.Since(startedAt).Milliseconds()))
	result.Processing[0].DurationMs = &duration
}

// recordFailure создаёт storage.NewReport с исходным путём, пустым TargetPath,
// ActionNotWritten, количеством уже полученных SeverityError и причиной сбоя,
// затем вызывает reporter.Record. Непроверенная модель не передаётся в Save.
func (r *runner) recordFailure(result model.Result, path string, cause error) {
	errorCount := 0
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.SeverityLevel == diagnostics.SeverityError {
			errorCount++
		}
	}
	report := storage.NewReport(path, "", storage.ActionNotWritten, errorCount, cause)
	r.reporter.Record(result, report)
}

// failedReader превращает ошибку os.Open в ошибку чтения для parser.New.
type failedReader struct {
	// err — исходная ошибка открытия источника для диагностики IO001.
	err error
}

// Read возвращает исходную ошибку без байтов при первом чтении парсером.
func (r failedReader) Read(p []byte) (n int, err error) {
	return 0, r.err
}
