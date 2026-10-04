// Package parser читает байтовый поток, вызывает выбранную построчную грамматику
// и дополняет переданную модель результатом разбора одного DSL-документа.
// Пакет отвечает за ранние диагностики, структурные связи и целостность модели;
// он не обходит файлы, не печатает сообщения и не сохраняет JSON.
package parser

import (
	"fmt"
	"io"
	"maps"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

// Parser хранит вход, модель и состояние разбора одного документа.
type Parser struct {
	// reader — байтовый поток этого документа; читается один раз.
	reader io.Reader
	// result — модель документа, принадлежащая вызывающему коду.
	result *model.Result
	// grammars — зарегистрированные наборы функций грамматик.
	grammars grammar.Registry
	// diagnostics — нормативные описания диагностик.
	diagnostics diagnostics.Registry
	// assertionFuncs — копия переданной карты: версия DSL, затем публичный ID
	// конкретной грамматики и функция проверки этого ID.
	assertionFuncs map[string]map[grammar.AssertionID]grammar.AssertionFunc
	// started — защита от повторного вызова Parse для этого reader и result.
	started bool
	// processingID — ID текущего запуска, уже записанный в result.Processing.
	processingID string
	// version — версия, выделенная до выбора грамматики.
	version string
	// detections — функции детекции выбранной грамматики в порядке регистрации.
	detections []grammar.DetectionFunc
	// orchestrators — функции оркестрации в порядке регистрации.
	orchestrators []grammar.OrchestrationFunc
	// assertions — функции по ID для выбранной версии, без второй регистрации
	// функций в реестре грамматик.
	assertions map[grammar.AssertionID]grammar.AssertionFunc
	// enterParentID и leaveParentID — ID структурных вопросов, полученные
	// из выбранной записи реестра грамматик, а не из порядка обхода карты.
	enterParentID grammar.AssertionID
	leaveParentID grammar.AssertionID
	// openBlocks — номера строк ещё открытых фигурных блоков.
	openBlocks []int
	// parents — номера активных строк логических родителей.
	parents []int
	// seenMetadata — номер первого объявления каждого метатега или заголовка.
	seenMetadata map[string]int
	// nextDiagnosticID — кандидат для уникального ID с учётом уже существующих
	// диагностик result; совпадения с прежними ID пропускаются.
	nextDiagnosticID int
}

// New создаёт парсер одного документа, закрепляет reader и result и копирует
// оба уровня карты утверждений. Функции по ID предоставляет вызывающий код;
// версия источника выбирается после чтения и проверки байтов.
func New(reader io.Reader, result *model.Result, grammars grammar.Registry, registry diagnostics.Registry, assertionFuncs map[string]map[grammar.AssertionID]grammar.AssertionFunc) *Parser {
	copiedAssertions := maps.Clone(assertionFuncs)
	for version, assertions := range copiedAssertions {
		copiedAssertions[version] = maps.Clone(assertions)
	}
	return &Parser{
		reader: reader, result: result, grammars: grammars, diagnostics: registry,
		assertionFuncs: copiedAssertions, seenMetadata: make(map[string]int), nextDiagnosticID: 1,
	}
}

// Parse один раз читает закреплённый reader и дополняет закреплённый result.
// Текущий запуск — последняя запись result.Processing с готовым ID.
// Ошибки входа становятся диагностиками в result, а ошибка метода означает
// повторный вызов, некорректную модель, конфигурацию или внутренний инвариант.
func (p *Parser) Parse() error {
	if p == nil {
		return fmt.Errorf("парсер отсутствует")
	}
	if p.started {
		return fmt.Errorf("повторный вызов Parse для одного документа")
	}
	p.started = true
	if err := p.prepareResult(); err != nil {
		return err
	}
	if p.reader == nil {
		return fmt.Errorf("не задан поток документа")
	}
	if p.diagnostics == nil {
		return fmt.Errorf("не задан реестр диагностик")
	}
	lines, ready, err := p.prepareInput()
	if err == nil && ready {
		err = p.parseLines(lines)
		if err == nil {
			err = p.checkOpenBlocks()
		}
	}
	orderDiagnostics(p.result)
	setErrorFlags(p.result)
	if err != nil {
		return err
	}
	return validateResult(p.result)
}

// prepareResult проверяет текущий Processing, пустой набор ещё не разобранных
// Lines, Format и существующие ID; заполняет обязательные пустые срезы модели,
// не удаляя прежние Processing и Diagnostics.
func (p *Parser) prepareResult() error {
	id, err := currentProcessingID(p.result)
	if err != nil {
		return err
	}
	if p.result.Format != model.FormatVersion1 {
		return fmt.Errorf("неподдерживаемый формат модели %q", p.result.Format)
	}
	if len(p.result.Lines) != 0 {
		return fmt.Errorf("модель уже содержит строки документа")
	}
	if err := checkDiagnosticSources(p.result); err != nil {
		return err
	}
	ids := make(map[string]bool, len(p.result.Diagnostics))
	for _, diagnostic := range p.result.Diagnostics {
		if diagnostic.ID == "" || ids[diagnostic.ID] {
			return fmt.Errorf("пустой или повторный Diagnostic.ID %q", diagnostic.ID)
		}
		ids[diagnostic.ID] = true
	}
	if p.result.Lines == nil {
		p.result.Lines = []model.Line{}
	}
	if p.result.Diagnostics == nil {
		p.result.Diagnostics = []model.Diagnostic{}
	}
	if p.result.Document.Metadata.ResourceDirs == nil {
		p.result.Document.Metadata.ResourceDirs = []string{}
	}
	for i := range p.result.Diagnostics {
		if p.result.Diagnostics[i].RelatedLocations == nil {
			p.result.Diagnostics[i].RelatedLocations = []model.Location{}
		}
	}
	p.processingID = id
	return nil
}

// currentProcessingID проверяет существование и уникальность ID последней
// записи Processing и возвращает его для Diagnostic.Source.
func currentProcessingID(result *model.Result) (id string, err error) {
	if result == nil {
		return "", fmt.Errorf("модель результата отсутствует")
	}
	if len(result.Processing) == 0 {
		return "", fmt.Errorf("не задан текущий запуск Processing")
	}
	id = result.Processing[len(result.Processing)-1].ID
	if id == "" {
		return "", fmt.Errorf("текущий Processing.ID пуст")
	}
	for _, previous := range result.Processing[:len(result.Processing)-1] {
		if previous.ID == id {
			return "", fmt.Errorf("текущий Processing.ID %q повторяется", id)
		}
	}
	return id, nil
}
