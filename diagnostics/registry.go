package diagnostics

// Registry предоставляет поиск неизменяемых описаний диагностик по коду.
type Registry interface {
	// Lookup возвращает описание диагностики по коду.
	// found равен false, если код не зарегистрирован.
	Lookup(code Code) (description Diagnostic, found bool)
}

// registry хранит внутреннюю карту описаний диагностик.
type registry struct {
	// diagnostics сопоставляет код с неизменяемым описанием диагностики.
	diagnostics map[Code]diagnostic
}

// NewRegistry создаёт реестр и регистрирует все известные пакету диагностики.
// Встроенный набор включает P001–P015 и IO001.
func NewRegistry() Registry {
	descriptions := builtinDiagnostics()
	r := registry{diagnostics: make(map[Code]diagnostic, len(descriptions))}
	for _, description := range descriptions {
		r.diagnostics[description.code] = description
	}
	return r
}

// Lookup возвращает описание диагностики по её коду.
func (r registry) Lookup(code Code) (description Diagnostic, found bool) {
	descriptionValue, found := r.diagnostics[code]
	if !found {
		return nil, false
	}
	return descriptionValue, true
}
