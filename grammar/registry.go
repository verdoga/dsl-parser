package grammar

import "slices"

// Registry сопоставляет строку версии DSL с набором функций её грамматики.
type Registry map[string]grammarCollection

// NewRegistry создаёт пустой реестр грамматик.
func NewRegistry() Registry {
	return make(Registry)
}

// Register сохраняет набор функций грамматики для версии DSL.
// Повторная регистрация той же версии заменяет ранее сохранённый набор.
// Все переданные срезы копируются с сохранением порядка элементов.
func (r Registry) Register(
	version string,
	detectionFuncs []DetectionFunc,
	orchestrationFuncs []OrchestrationFunc,
	lineTypeFuncs []LineTypeFunc,
	lineParserFuncs []LineParserFunc,
	assertions []Assertion,
) {
	r[version] = grammarCollection{
		version:            version,
		detectionFuncs:     slices.Clone(detectionFuncs),
		orchestrationFuncs: slices.Clone(orchestrationFuncs),
		lineTypeFuncs:      slices.Clone(lineTypeFuncs),
		lineParserFuncs:    slices.Clone(lineParserFuncs),
		assertions:         slices.Clone(assertions),
	}
}

// Lookup возвращает набор функций грамматики для версии DSL.
// found равен false, если версия не зарегистрирована.
func (r Registry) Lookup(version string) (grammar grammarCollection, found bool) {
	grammar, found = r[version]
	return grammar, found
}
