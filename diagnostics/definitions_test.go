package diagnostics

import "testing"

func TestBuiltinDiagnosticsProvidesCompleteDescriptions(t *testing.T) {
	tests := []struct {
		code     Code
		message  string
		scope    Scope
		fatal    bool
		hasCheck bool
	}{
		{
			code: P001, message: "Некорректная кодировка UTF-8",
			scope: ScopeDocument, fatal: true, hasCheck: true,
		},
		{
			code: P002, message: "Неподдерживаемый перевод строки",
			scope: ScopeDocument, fatal: true, hasCheck: true,
		},
		{
			code: P003, message: "Неизвестная DSL-конструкция",
			scope: ScopeElement,
		},
		{
			code: P004, message: "Нарушен обязательный разделитель объявления",
			scope: ScopeLine,
		},
		{
			code: P005, message: "Использована неподдерживаемая форма тега",
			scope: ScopeLine,
		},
		{
			code: P006, message: "Отсутствует обязательный структурный параметр",
			scope: ScopeLine,
		},
		{
			code: P007, message: "Лишние параметры или запрещённое содержимое объявления",
			scope: ScopeLine,
		},
		{
			code: P008, message: "Неправильное открытие блока",
			scope: ScopeLine,
		},
		{
			code: P009, message: "Закрытие несуществующего блока",
			scope: ScopeLine,
		},
		{
			code: P010, message: "Неправильная строка закрытия блока",
			scope: ScopeLine,
		},
		{
			code: P011, message: "Незакрытый блок",
			scope: ScopeBlock,
		},
		{
			code: P012, message: "Неэкранированная фигурная скобка в текстовом значении",
			scope: ScopeElement,
		},
		{
			code: P013, message: "Не удалось получить объявление версии",
			scope: ScopeDocument, fatal: true, hasCheck: true,
		},
		{
			code: P014, message: "Версия не поддерживается",
			scope: ScopeDocument, fatal: true, hasCheck: true,
		},
		{
			code: P015, message: "Незавершённый разбор строки",
			scope: ScopeElement, fatal: false, hasCheck: false,
		},
		{
			code: IO001, message: "Техническая ошибка чтения входного файла",
			scope: ScopeDocument, fatal: true, hasCheck: true,
		},
	}

	descriptions := builtinDiagnostics()
	if len(descriptions) != len(tests) {
		t.Errorf("builtinDiagnostics() вернул %d описаний, требуется %d", len(descriptions), len(tests))
	}
	byCode := make(map[Code]Diagnostic, len(descriptions))
	for _, description := range descriptions {
		code := description.Code()
		if _, exists := byCode[code]; exists {
			t.Errorf("builtinDiagnostics() вернул повторяющийся код %q", code)
		}
		byCode[code] = description
	}

	for _, test := range tests {
		t.Run(string(test.code), func(t *testing.T) {
			description, found := byCode[test.code]
			if !found {
				t.Fatalf("builtinDiagnostics() не содержит код %q", test.code)
			}
			if got := description.Severity(); got != SeverityError {
				t.Errorf("Severity() = %q, требуется %q", got, SeverityError)
			}
			if got := description.Message(); got != test.message {
				t.Errorf("Message() = %q, требуется %q", got, test.message)
			}
			if got := description.Scope(); got != test.scope {
				t.Errorf("Scope() = %q, требуется %q", got, test.scope)
			}
			if got := description.IsFatal(); got != test.fatal {
				t.Errorf("IsFatal() = %t, требуется %t", got, test.fatal)
			}
			if got := description.HasCheck(); got != test.hasCheck {
				t.Errorf("HasCheck() = %t, требуется %t", got, test.hasCheck)
			}
		})
	}
}
