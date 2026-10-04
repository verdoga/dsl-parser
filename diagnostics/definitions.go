package diagnostics

// builtinDiagnostics возвращает полный набор встроенных описаний P001–P015 и IO001
// для регистрации конструктором NewRegistry.
func builtinDiagnostics() []diagnostic {
	return []diagnostic{
		{
			code:     P001,
			severity: SeverityError,
			message:  "Некорректная кодировка UTF-8",
			scope:    ScopeDocument,
			fatal:    true,
			check:    checkP001,
		},
		{
			code:     P002,
			severity: SeverityError,
			message:  "Неподдерживаемый перевод строки",
			scope:    ScopeDocument,
			fatal:    true,
			check:    checkP002,
		},
		{
			code:     P003,
			severity: SeverityError,
			message:  "Неизвестная DSL-конструкция",
			scope:    ScopeElement,
		},
		{
			code:     P004,
			severity: SeverityError,
			message:  "Нарушен обязательный разделитель объявления",
			scope:    ScopeLine,
		},
		{
			code:     P005,
			severity: SeverityError,
			message:  "Использована неподдерживаемая форма тега",
			scope:    ScopeLine,
		},
		{
			code:     P006,
			severity: SeverityError,
			message:  "Отсутствует обязательный структурный параметр",
			scope:    ScopeLine,
		},
		{
			code:     P007,
			severity: SeverityError,
			message:  "Лишние параметры или запрещённое содержимое объявления",
			scope:    ScopeLine,
		},
		{
			code:     P008,
			severity: SeverityError,
			message:  "Неправильное открытие блока",
			scope:    ScopeLine,
		},
		{
			code:     P009,
			severity: SeverityError,
			message:  "Закрытие несуществующего блока",
			scope:    ScopeLine,
		},
		{
			code:     P010,
			severity: SeverityError,
			message:  "Неправильная строка закрытия блока",
			scope:    ScopeLine,
		},
		{
			code:     P011,
			severity: SeverityError,
			message:  "Незакрытый блок",
			scope:    ScopeBlock,
		},
		{
			code:     P012,
			severity: SeverityError,
			message:  "Неэкранированная фигурная скобка в текстовом значении",
			scope:    ScopeElement,
		},
		{
			code:     P013,
			severity: SeverityError,
			message:  "Не удалось получить объявление версии",
			scope:    ScopeDocument,
			fatal:    true,
			check:    checkP013,
		},
		{
			code:     P014,
			severity: SeverityError,
			message:  "Версия не поддерживается",
			scope:    ScopeDocument,
			fatal:    true,
			check:    checkP014,
		},
		{
			code:     P015,
			severity: SeverityError,
			message:  "Незавершённый разбор строки",
			scope:    ScopeElement,
		},
		{
			code:     IO001,
			severity: SeverityError,
			message:  "Техническая ошибка чтения входного файла",
			scope:    ScopeDocument,
			fatal:    true,
			check:    checkIO001,
		},
	}
}
