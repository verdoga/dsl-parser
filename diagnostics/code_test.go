package diagnostics

import "testing"

func TestCodesHaveStableUniqueValues(t *testing.T) {
	tests := []struct {
		name string
		code Code
		want string
	}{
		{name: "P001", code: P001, want: "P001"},
		{name: "P002", code: P002, want: "P002"},
		{name: "P003", code: P003, want: "P003"},
		{name: "P004", code: P004, want: "P004"},
		{name: "P005", code: P005, want: "P005"},
		{name: "P006", code: P006, want: "P006"},
		{name: "P007", code: P007, want: "P007"},
		{name: "P008", code: P008, want: "P008"},
		{name: "P009", code: P009, want: "P009"},
		{name: "P010", code: P010, want: "P010"},
		{name: "P011", code: P011, want: "P011"},
		{name: "P012", code: P012, want: "P012"},
		{name: "P013", code: P013, want: "P013"},
		{name: "P014", code: P014, want: "P014"},
		{name: "P015", code: P015, want: "P015"},
		{name: "IO001", code: IO001, want: "IO001"},
	}

	seen := make(map[Code]string, len(tests))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if string(test.code) != test.want {
				t.Errorf("%s = %q, требуется %q", test.name, test.code, test.want)
			}
			if previous, exists := seen[test.code]; exists {
				t.Errorf("%s и %s имеют одинаковое значение %q", previous, test.name, test.code)
			}
			seen[test.code] = test.name
		})
	}
}

func TestSeveritiesHaveStableUniqueValues(t *testing.T) {
	tests := []struct {
		name     string
		severity Severity
		want     string
	}{
		{name: "SeverityError", severity: SeverityError, want: "error"},
		{name: "SeverityWarning", severity: SeverityWarning, want: "warning"},
		{name: "SeverityRecommendation", severity: SeverityRecommendation, want: "recommendation"},
	}

	seen := make(map[Severity]string, len(tests))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if string(test.severity) != test.want {
				t.Errorf("%s = %q, требуется %q", test.name, test.severity, test.want)
			}
			if previous, exists := seen[test.severity]; exists {
				t.Errorf("%s и %s имеют одинаковое значение %q", previous, test.name, test.severity)
			}
			seen[test.severity] = test.name
		})
	}
}

func TestScopesHaveStableUniqueValues(t *testing.T) {
	tests := []struct {
		name  string
		scope Scope
		want  string
	}{
		{name: "ScopeElement", scope: ScopeElement, want: "element"},
		{name: "ScopeLine", scope: ScopeLine, want: "line"},
		{name: "ScopeBlock", scope: ScopeBlock, want: "block"},
		{name: "ScopeDocument", scope: ScopeDocument, want: "document"},
	}

	seen := make(map[Scope]string, len(tests))
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if string(test.scope) != test.want {
				t.Errorf("%s = %q, требуется %q", test.name, test.scope, test.want)
			}
			if previous, exists := seen[test.scope]; exists {
				t.Errorf("%s и %s имеют одинаковое значение %q", previous, test.name, test.scope)
			}
			seen[test.scope] = test.name
		})
	}
}
