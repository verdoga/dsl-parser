package diagnostics

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestDiagnosticReturnsStoredProperties(t *testing.T) {
	tests := []struct {
		name     string
		code     Code
		severity Severity
		message  string
		scope    Scope
		fatal    bool
	}{
		{name: "zero values"},
		{
			name: "fatal document error", code: P001, severity: SeverityError,
			message: "Некорректная кодировка UTF-8.", scope: ScopeDocument, fatal: true,
		},
		{
			name: "element warning", code: Code("TEST-WARNING"), severity: SeverityWarning,
			message: "Предупреждение об элементе.", scope: ScopeElement,
		},
		{
			name: "line recommendation", code: Code("TEST-RECOMMENDATION"), severity: SeverityRecommendation,
			message: "Рекомендация для строки.", scope: ScopeLine,
		},
		{
			name: "nonfatal block error", code: P011, severity: SeverityError,
			message: "Незакрытый блок.", scope: ScopeBlock,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var description Diagnostic = diagnostic{
				code: test.code, severity: test.severity, message: test.message,
				scope: test.scope, fatal: test.fatal,
			}
			if got := description.Code(); got != test.code {
				t.Errorf("Code() = %q, требуется %q", got, test.code)
			}
			if got := description.Severity(); got != test.severity {
				t.Errorf("Severity() = %q, требуется %q", got, test.severity)
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
		})
	}
}

func TestDiagnosticHasCheckReportsRegistrationWithoutCallingCheck(t *testing.T) {
	tests := []struct {
		name       string
		registered bool
	}{
		{name: "unavailable"},
		{name: "registered", registered: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			var check CheckFunc
			if test.registered {
				check = func(io.Reader) (bool, error) {
					calls++
					return false, nil
				}
			}
			var description Diagnostic = diagnostic{check: check}
			if got := description.HasCheck(); got != test.registered {
				t.Errorf("HasCheck() = %t, требуется %t", got, test.registered)
			}
			if calls != 0 {
				t.Errorf("HasCheck() вызвал проверку %d раз, требуется 0", calls)
			}
		})
	}
}

func TestDiagnosticCheckUnavailableDoesNotRead(t *testing.T) {
	reader := &diagnosticCountingReader{}
	var description Diagnostic = diagnostic{}

	matched, err := description.Check(reader)
	if matched {
		t.Error("Check() вернул matched=true при отсутствии функции проверки")
	}
	if err != ErrCheckUnavailable {
		t.Errorf("Check() вернул ошибку %v, требуется непосредственно ErrCheckUnavailable", err)
	}
	if reader.reads != 0 {
		t.Errorf("Check() вызвал Read() %d раз при отсутствии функции проверки, требуется 0", reader.reads)
	}
}

func TestDiagnosticCheckForwardsReaderAndResults(t *testing.T) {
	checkErr := errors.New("техническая ошибка проверки")
	tests := []struct {
		name    string
		matched bool
		err     error
	}{
		{name: "not matched"},
		{name: "matched", matched: true},
		{name: "not matched with error", err: checkErr},
		{name: "matched with error", matched: true, err: checkErr},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := strings.NewReader("исходный поток")
			calls := 0
			var description Diagnostic = diagnostic{
				check: func(gotReader io.Reader) (bool, error) {
					calls++
					if gotReader != reader {
						t.Error("функция проверки получила другой читатель")
					}
					return test.matched, test.err
				},
			}

			for call := 1; call <= 2; call++ {
				matched, err := description.Check(reader)
				if calls != call {
					t.Errorf("после %d вызовов Check() функция вызвана %d раз", call, calls)
				}
				if matched != test.matched {
					t.Errorf("Check() вернул matched=%t, требуется %t", matched, test.matched)
				}
				if err != test.err {
					t.Errorf("Check() вернул ошибку %v, требуется исходная ошибка %v", err, test.err)
				}
			}
		})
	}
}

type diagnosticCountingReader struct {
	reads int
}

func (r *diagnosticCountingReader) Read([]byte) (int, error) {
	r.reads++
	return 0, io.EOF
}
