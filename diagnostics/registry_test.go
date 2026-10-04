package diagnostics

import "testing"

func TestRegistryLookupFindsAllBuiltinCodes(t *testing.T) {
	codes := []Code{
		P001, P002, P003, P004, P005, P006, P007, P008,
		P009, P010, P011, P012, P013, P014, IO001,
	}
	r := NewRegistry()

	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			description, found := r.Lookup(code)
			if !found {
				t.Fatalf("Lookup(%q) вернул found=false для встроенного кода", code)
			}
			if description == nil {
				t.Fatalf("Lookup(%q) вернул nil вместо описания", code)
			}
			if got := description.Code(); got != code {
				t.Errorf("Lookup(%q) вернул описание с кодом %q", code, got)
			}
		})
	}
}

func TestRegistryLookupRejectsUnregisteredCodes(t *testing.T) {
	tests := []struct {
		name string
		code Code
	}{
		{name: "empty"},
		{name: "unknown parser code", code: "P015"},
		{name: "unknown IO code", code: "IO002"},
		{name: "arbitrary code", code: "UNKNOWN"},
		{name: "lowercase parser code", code: "p001"},
		{name: "lowercase IO code", code: "io001"},
		{name: "mixed case IO code", code: "Io001"},
		{name: "leading space", code: " P001"},
		{name: "trailing space", code: "P001 "},
		{name: "surrounding spaces", code: " IO001 "},
		{name: "leading tab", code: "\tP001"},
		{name: "trailing newline", code: "P001\n"},
	}
	r := NewRegistry()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			description, found := r.Lookup(test.code)
			if found {
				t.Errorf("Lookup(%q) вернул found=true для незарегистрированного кода", test.code)
			}
			if description != nil {
				t.Errorf("Lookup(%q) вернул описание %v, требуется nil", test.code, description)
			}
		})
	}
}

func TestRegistryLookupIsStableAcrossCallsAndRegistries(t *testing.T) {
	codes := []Code{
		P001, P002, P003, P004, P005, P006, P007, P008,
		P009, P010, P011, P012, P013, P014, IO001,
	}
	first := NewRegistry()
	second := NewRegistry()
	tests := []struct {
		name     string
		registry Registry
	}{
		{name: "same registry", registry: first},
		{name: "another registry", registry: second},
	}

	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			want, found := first.Lookup(code)
			if !found || want == nil {
				t.Fatalf("первый Lookup(%q) не вернул описание", code)
			}
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					for attempt := 1; attempt <= 2; attempt++ {
						got, found := test.registry.Lookup(code)
						if !found {
							t.Fatalf("Lookup(%q), попытка %d: found=false", code, attempt)
						}
						assertRegistryDescriptionMatches(t, got, want)
					}
				})
			}
		})
	}
}

func assertRegistryDescriptionMatches(t *testing.T, got, want Diagnostic) {
	t.Helper()
	if got == nil {
		t.Fatal("Lookup() вернул nil вместо описания")
	}
	if got.Code() != want.Code() {
		t.Errorf("Code() = %q, требуется %q", got.Code(), want.Code())
	}
	if got.Severity() != want.Severity() {
		t.Errorf("Severity() = %q, требуется %q", got.Severity(), want.Severity())
	}
	if got.Message() != want.Message() {
		t.Errorf("Message() = %q, требуется %q", got.Message(), want.Message())
	}
	if got.Scope() != want.Scope() {
		t.Errorf("Scope() = %q, требуется %q", got.Scope(), want.Scope())
	}
	if got.IsFatal() != want.IsFatal() {
		t.Errorf("IsFatal() = %t, требуется %t", got.IsFatal(), want.IsFatal())
	}
	if got.HasCheck() != want.HasCheck() {
		t.Errorf("HasCheck() = %t, требуется %t", got.HasCheck(), want.HasCheck())
	}
}
