package grammar

import (
	"fmt"
	"slices"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
)

// GrammarContext предоставляет функциям грамматики исходную строку,
// результаты обработки и операции накопления этих результатов.
type GrammarContext interface {
	// fmt.Stringer возвращает исходную строку без BOM и окончания строки.
	fmt.Stringer

	// diagnostics.Registry предоставляет поиск описания диагностики по коду.
	// Перед поиском вызывающий код обязан установить реестр.
	diagnostics.Registry

	// SetLine сохраняет результат обработки текущей строки.
	SetLine(line model.Line)

	// AddElement добавляет элемент в результат обработки текущей строки.
	AddElement(element model.Element)

	// Line возвращает полный результат обработки текущей строки.
	Line() model.Line

	// AddDiagnostic добавляет диагностику обработки текущей строки.
	AddDiagnostic(diagnostic model.Diagnostic)

	// Diagnostics возвращает все накопленные диагностики текущей строки.
	// Возвращаемый срез не разделяет хранилище с контекстом.
	Diagnostics() []model.Diagnostic

	// SetDiagnosticRegistry устанавливает реестр описаний диагностик.
	SetDiagnosticRegistry(registry diagnostics.Registry)

	// SetAssertions устанавливает утверждения выбранной версии грамматики.
	// Переданный срез копируется; повторный вызов заменяет предыдущий набор.
	SetAssertions(assertions []Assertion)

	// Assert вызывает утверждение по идентификатору с переданными данными.
	// true означает подтверждение утверждения; false — отрицательный ответ,
	// неизвестный идентификатор либо отсутствие функции проверки.
	// При повторяющемся идентификаторе используется первое совпадение.
	Assert(id AssertionID, input AssertionInput) bool

	// AddDetection добавляет количественный результат детектирования с указанным kind.
	// Повторный вызов с тем же kind заменяет предыдущее значение.
	AddDetection(kind string, detected int)

	// Detection возвращает количественный результат детектирования с указанным kind.
	// found равен false, если результат с таким kind отсутствует.
	Detection(kind string) (detected int, found bool)
}

// context хранит изменяемые данные обработки одной физической строки.
type context struct {
	// source — исходная строка без BOM и окончания строки.
	source string

	// line — накопленный результат обработки строки.
	line model.Line

	// diagnostics — накопленные диагностики строки.
	diagnostics []model.Diagnostic

	// Registry предоставляет поиск описания диагностики по коду.
	diagnostics.Registry

	// assertions — проверки выбранной грамматики, сохраняемые между строками.
	// Для каждой строки вызывающий код устанавливает набор в новом контексте.
	assertions []Assertion

	// detections — накопленные результаты детектирования.
	detections []detection
}

// detection хранит один результат детектирующей функции.
type detection struct {
	// kind — идентификатор детектирующей функции.
	kind string

	// detected — количественный результат детектирования.
	detected int
}

// NewContext создаёт контекст для обработки одной физической строки.
// source не содержит BOM и окончания строки.
// Перед поиском описаний диагностик необходимо установить их реестр.
func NewContext(source string) GrammarContext {
	return &context{source: source}
}

// String возвращает исходную строку контекста.
func (c *context) String() string {
	return c.source
}

// SetLine сохраняет результат обработки текущей строки.
// Вложенные срезы и указатели модели не копируются.
func (c *context) SetLine(line model.Line) {
	c.line = line
}

// AddElement добавляет элемент в результат обработки текущей строки.
func (c *context) AddElement(element model.Element) {
	c.line.Elements = append(c.line.Elements, element)
}

// Line возвращает полный результат обработки текущей строки.
// Вложенные срезы и указатели модели не копируются.
func (c *context) Line() model.Line {
	return c.line
}

// AddDiagnostic добавляет диагностику обработки текущей строки.
func (c *context) AddDiagnostic(diagnostic model.Diagnostic) {
	c.diagnostics = append(c.diagnostics, diagnostic)
}

// Diagnostics возвращает все накопленные диагностики текущей строки.
// Возвращаемый срез не разделяет хранилище с контекстом;
// вложенные срезы и указатели модели не копируются.
func (c *context) Diagnostics() []model.Diagnostic {
	return slices.Clone(c.diagnostics)
}

// SetDiagnosticRegistry устанавливает реестр описаний диагностик.
func (c *context) SetDiagnosticRegistry(registry diagnostics.Registry) {
	c.Registry = registry
}

// SetAssertions устанавливает копию набора утверждений выбранной грамматики.
func (c *context) SetAssertions(assertions []Assertion) {
	c.assertions = slices.Clone(assertions)
}

// Assert вызывает утверждение по идентификатору с переданными данными.
// При неизвестном идентификаторе или отсутствии функции возвращает false.
// При повторяющемся идентификаторе используется первое совпадение.
func (c *context) Assert(id AssertionID, input AssertionInput) bool {
	for _, assertion := range c.assertions {
		if assertion.ID == id {
			return assertion.Check != nil && assertion.Check(input)
		}
	}
	return false
}

// AddDetection добавляет количественный результат детектирования с указанным kind.
// Повторный вызов с тем же kind заменяет предыдущее значение.
func (c *context) AddDetection(kind string, detected int) {
	for i := range c.detections {
		if c.detections[i].kind == kind {
			c.detections[i].detected = detected
			return
		}
	}
	c.detections = append(c.detections, detection{kind: kind, detected: detected})
}

// Detection возвращает количественный результат детектирования с указанным kind.
// found равен false, если результат с таким kind отсутствует.
func (c *context) Detection(kind string) (detected int, found bool) {
	for _, result := range c.detections {
		if result.kind == kind {
			return result.detected, true
		}
	}
	return 0, false
}
