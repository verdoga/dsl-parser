// Package grammar предоставляет реестр наборов функций грамматик DSL
// и общий контекст обработки одной физической строки.
//
// Пакет не содержит конкретных грамматик, не выбирает версию из исходного
// текста, не разбирает DSL и не управляет разбором документа. Отдельные
// пакеты грамматик регистрируют в нём свои наборы функций, а парсер получает
// набор по уже прочитанной версии DSL.
package grammar

import (
	"slices"

	"github.com/verdoga/dsl-parser/model"
)

// DetectionFunc выполняет одно действие этапа детектирования.
// true означает успех действия, false — неуспех действия.
type DetectionFunc func(context GrammarContext) bool

// OrchestrationFunc по накопленным результатам детектирования выбирает
// и запускает необходимые действия определения типа или разбора строки.
// true означает успех действия, false — неуспех действия.
type OrchestrationFunc func(context GrammarContext) bool

// LineTypeFunc выполняет одно действие этапа определения типа строки.
// true означает успех действия, false — неуспех действия.
type LineTypeFunc func(context GrammarContext) bool

// LineParserFunc выполняет одно действие этапа разбора строки.
// true означает успех действия, false — неуспех действия.
type LineParserFunc func(context GrammarContext) bool

// AssertionID — идентификатор одного вопроса к грамматике.
// Публичные идентификаторы объявляет пакет конкретной версии DSL.
type AssertionID string

// AssertionInput содержит типизированные данные для проверки утверждения.
// Line — проверяемая строка до назначения ей логического родителя.
// OpenBlock и CandidateParent могут отсутствовать; значение каждого поля
// определяется состоянием парсера на момент вызова.
type AssertionInput struct {
	Line            model.Line
	OpenBlock       *model.Line
	CandidateParent *model.Line
}

// AssertionFunc отвечает на один вопрос к грамматике без изменения входных
// данных, контекста строки или состояния парсера.
// true означает, что утверждение выполняется; false — что не выполняется.
type AssertionFunc func(input AssertionInput) bool

// Assertion связывает публичный идентификатор вопроса с его проверкой.
type Assertion struct {
	ID    AssertionID
	Check AssertionFunc
}

// grammarCollection хранит версию DSL и упорядоченные наборы функций
// четырёх этапов её грамматики, а также именованные утверждения.
type grammarCollection struct {
	// version — версия DSL, для которой зарегистрирован набор функций.
	version string

	// detectionFuncs — функции этапа детектирования в порядке их вызова.
	detectionFuncs []DetectionFunc

	// orchestrationFuncs — функции этапа оркестрации в порядке их вызова.
	orchestrationFuncs []OrchestrationFunc

	// lineTypeFuncs — функции этапа определения типа строки в порядке их вызова.
	lineTypeFuncs []LineTypeFunc

	// lineParserFuncs — функции этапа разбора строки в порядке их вызова.
	lineParserFuncs []LineParserFunc

	// assertions — именованные проверки грамматики.
	assertions []Assertion
}

// Version возвращает версию DSL, которой соответствует набор функций.
func (g grammarCollection) Version() string {
	return g.version
}

// DetectionFuncs возвращает упорядоченный набор функций детектирования.
// Возвращаемый срез не разделяет хранилище с набором грамматики.
func (g grammarCollection) DetectionFuncs() []DetectionFunc {
	return slices.Clone(g.detectionFuncs)
}

// OrchestrationFuncs возвращает упорядоченный набор функций оркестрации.
// Возвращаемый срез не разделяет хранилище с набором грамматики.
func (g grammarCollection) OrchestrationFuncs() []OrchestrationFunc {
	return slices.Clone(g.orchestrationFuncs)
}

// LineTypeFuncs возвращает упорядоченный набор функций определения типа строки.
// Возвращаемый срез не разделяет хранилище с набором грамматики.
func (g grammarCollection) LineTypeFuncs() []LineTypeFunc {
	return slices.Clone(g.lineTypeFuncs)
}

// LineParserFuncs возвращает упорядоченный набор функций разбора строки.
// Возвращаемый срез не разделяет хранилище с набором грамматики.
func (g grammarCollection) LineParserFuncs() []LineParserFunc {
	return slices.Clone(g.lineParserFuncs)
}

// Assertions возвращает зарегистрированные именованные проверки.
// Возвращаемый срез не разделяет хранилище с набором грамматики.
func (g grammarCollection) Assertions() []Assertion {
	return slices.Clone(g.assertions)
}
