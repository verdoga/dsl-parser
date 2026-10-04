package parser

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
)

func TestValidateResultAcceptsValidModelsWithoutMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*model.Result)
	}{
		{"Unicode and mixed EOL", func(*model.Result) {}},
		{"last LF", func(r *model.Result) { r.Lines[3].LineEnding = model.LineEndingLF }},
		{"empty physical line", func(r *model.Result) {
			r.Lines[0].Raw, r.Lines[0].LineType, r.Lines[0].Elements = "", model.LineTypeBlank, []model.Element{}
		}},
		{"empty document", func(r *model.Result) {
			r.Lines, r.Diagnostics = []model.Line{}, []model.Diagnostic{}
			*r.Document.LineCount = 0
			r.Document.HasErrors = false
		}},
		{"insertion at Unicode EOF", func(r *model.Result) {
			position := model.Position{Line: 3, Column: 3}
			r.Lines[2].Elements = append(r.Lines[2].Elements, model.Element{
				ElementType: model.ElementTypeUnparsed, Start: 3, End: 3, ErrorIDs: []string{"insert"},
			})
			r.Diagnostics = append(r.Diagnostics, model.Diagnostic{
				ID: "insert", Source: "run-b", DiagnosticCode: diagnostics.P015, SeverityLevel: diagnostics.SeverityError,
				DiagnosticScope: diagnostics.ScopeElement, Location: &model.Location{Start: position, End: position},
			})
		}},
		{"unclosed block with P011", func(r *model.Result) {
			r.Lines = r.Lines[:3]
			*r.Document.LineCount = 3
			r.Lines[1].HasErrors = true
			r.Diagnostics = append(r.Diagnostics, model.Diagnostic{
				ID: "open", Source: "run-b", DiagnosticCode: diagnostics.P011, SeverityLevel: diagnostics.SeverityError,
				DiagnosticScope: diagnostics.ScopeBlock,
				Location:        &model.Location{Start: model.Position{Line: 2, Column: 7}, End: model.Position{Line: 3, Column: 3}},
			})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := integrityResult(t)
			test.change(result)
			assertIntegrityResult(t, result, false)
		})
	}
}

func TestValidateResultAcceptsEarlyDiagnosticWithoutLines(t *testing.T) {
	for _, code := range []diagnostics.Code{diagnostics.IO001, diagnostics.P001, diagnostics.P002, diagnostics.P013, diagnostics.P014} {
		t.Run(string(code), func(t *testing.T) {
			byteLength := int64(42)
			result := &model.Result{
				Format: model.FormatVersion1, Document: model.Document{ByteLength: &byteLength, HasErrors: true},
				Processing: []model.Processing{{ID: "run"}}, Lines: []model.Line{},
				Diagnostics: []model.Diagnostic{{ID: "early", Source: "run", DiagnosticCode: code,
					SeverityLevel: diagnostics.SeverityError, DiagnosticScope: diagnostics.ScopeDocument, Fatal: true}},
			}
			assertIntegrityResult(t, result, false)
			result.Diagnostics[0].Location = &model.Location{Start: model.Position{Line: 1, Column: 1}, End: model.Position{Line: 1, Column: 1}}
			assertIntegrityResult(t, result, true)
		})
	}
}

func TestValidateResultRejectsIsolatedInvariantViolations(t *testing.T) {
	assertIntegrityResult(t, nil, true)
	for _, test := range []struct {
		name   string
		change func(*model.Result)
	}{
		{"line gap", func(r *model.Result) { r.Lines[2].Number = 8 }},
		{"duplicate line", func(r *model.Result) { r.Lines[2].Number = 2 }},
		{"line zero", func(r *model.Result) { r.Lines[0].Number = 0 }},
		{"wrong count", func(r *model.Result) { *r.Document.LineCount = 3 }},
		{"negative count", func(r *model.Result) { *r.Document.LineCount = -1 }},
		{"unknown count with lines", func(r *model.Result) { r.Document.LineCount = nil }},
		{"CR ending", func(r *model.Result) { r.Lines[0].LineEnding = "\r" }},
		{"unknown ending", func(r *model.Result) { r.Lines[0].LineEnding = "x" }},
		{"missing intermediate EOL", func(r *model.Result) { r.Lines[0].LineEnding = model.LineEndingNone }},
		{"LF in raw", func(r *model.Result) { r.Lines[0].Raw += "\n" }},
		{"CR in raw", func(r *model.Result) { r.Lines[0].Raw += "\r" }},
		{"initial BOM in raw", func(r *model.Result) { r.Lines[0].Raw = "\uFEFF" + r.Lines[0].Raw }},
		{"invalid UTF8", func(r *model.Result) { r.Lines[0].Raw = string([]byte{0xff}) }},
		{"phantom final line", func(r *model.Result) {
			r.Lines[3].LineEnding = model.LineEndingLF
			r.Lines = append(r.Lines, model.Line{Number: 5, LineType: model.LineTypeBlank})
			*r.Document.LineCount = 5
		}},
		{"missing parent", func(r *model.Result) { parent := 9; r.Lines[2].ParentLine = &parent }},
		{"future parent", func(r *model.Result) { parent := 4; r.Lines[2].ParentLine = &parent }},
		{"self parent", func(r *model.Result) { parent := 3; r.Lines[2].ParentLine = &parent }},
		{"parent zero", func(r *model.Result) { parent := 0; r.Lines[2].ParentLine = &parent }},
		{"wrong depth", func(r *model.Result) { r.Lines[2].NestingLevel = 2 }},
		{"negative depth", func(r *model.Result) { r.Lines[0].NestingLevel = -1 }},
		{"nonzero root depth", func(r *model.Result) { r.Lines[0].NestingLevel = 1 }},
		{"nested heading", func(r *model.Result) { r.Lines[2].LineType = model.LineTypeHeading }},
		{"closing points to content", func(r *model.Result) {
			parent := 3
			r.Lines[3].ParentLine, r.Lines[3].NestingLevel = &parent, 2
		}},
		{"closing misses opening", func(r *model.Result) { r.Lines[3].ParentLine, r.Lines[3].NestingLevel = nil, 0 }},
		{"empty processing ID", func(r *model.Result) { r.Processing[1].ID = "" }},
		{"duplicate processing ID", func(r *model.Result) { r.Processing[1].ID = "run-a" }},
		{"unknown source", func(r *model.Result) { r.Diagnostics[0].Source = "missing" }},
		{"empty source", func(r *model.Result) { r.Diagnostics[0].Source = "" }},
		{"empty diagnostic ID", func(r *model.Result) { r.Diagnostics[0].ID = "" }},
		{"duplicate diagnostic ID", func(r *model.Result) { r.Diagnostics[1].ID = "d1" }},
		{"range line zero", func(r *model.Result) { r.Diagnostics[0].Location.Start.Line = 0 }},
		{"range after EOF", func(r *model.Result) { r.Diagnostics[0].Location.End.Line = 5 }},
		{"column zero", func(r *model.Result) { r.Diagnostics[0].Location.Start.Column = 0 }},
		{"negative end column", func(r *model.Result) { r.Diagnostics[0].Location.End.Column = -1 }},
		{"byte column instead of Unicode", func(r *model.Result) { r.Diagnostics[0].Location.End.Column = 7 }},
		{"start beyond line", func(r *model.Result) { r.Diagnostics[0].Location.Start.Column = 4 }},
		{"reversed columns", func(r *model.Result) {
			r.Diagnostics[0].Location.Start.Column, r.Diagnostics[0].Location.End.Column = 3, 2
		}},
		{"reversed lines", func(r *model.Result) { r.Diagnostics[0].Location.End.Line = 2 }},
		{"multiline element", func(r *model.Result) { r.Diagnostics[0].Location.End.Line = 4 }},
		{"multiline line scope", func(r *model.Result) {
			r.Diagnostics[0].DiagnosticScope, r.Diagnostics[0].Location.End.Line = diagnostics.ScopeLine, 4
		}},
		{"related missing line", func(r *model.Result) { r.Diagnostics[0].RelatedLocations[0].Start.Line = 8 }},
		{"related column outside line", func(r *model.Result) { r.Diagnostics[0].RelatedLocations[0].End.Column = 9 }},
		{"duplicate related range", func(r *model.Result) {
			r.Diagnostics[0].RelatedLocations = append(r.Diagnostics[0].RelatedLocations, r.Diagnostics[0].RelatedLocations[0])
		}},
		{"related duplicates main", func(r *model.Result) { r.Diagnostics[0].RelatedLocations[0] = *r.Diagnostics[0].Location }},
		{"element without location", func(r *model.Result) { r.Diagnostics[0].Location = nil }},
		{"unknown element link", func(r *model.Result) { r.Lines[2].Elements[0].ErrorIDs = []string{"missing"} }},
		{"duplicate element link", func(r *model.Result) { r.Lines[2].Elements[0].ErrorIDs = []string{"d1", "d1", "d2"} }},
		{"reversed element links", func(r *model.Result) { r.Lines[2].Elements[0].ErrorIDs = []string{"d2", "d1"} }},
		{"missing backlink", func(r *model.Result) { r.Lines[2].Elements[0].ErrorIDs = []string{"d1"} }},
		{"mismatched range", func(r *model.Result) { r.Diagnostics[0].Location.Start.Column = 2 }},
		{"mismatched line", func(r *model.Result) { r.Diagnostics[0].Location.Start.Line, r.Diagnostics[0].Location.End.Line = 1, 1 }},
		{"link to line diagnostic", func(r *model.Result) { r.Diagnostics[0].DiagnosticScope = diagnostics.ScopeLine }},
		{"unparsed without error", func(r *model.Result) {
			r.Lines[0].Elements = []model.Element{{ElementType: model.ElementTypeUnparsed, Raw: "# A", Start: 1, End: 4}}
		}},
		{"invalid without unparsed", func(r *model.Result) { r.Lines[0].LineType = model.LineTypeInvalid }},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := integrityResult(t)
			test.change(result)
			assertIntegrityResult(t, result, true)
		})
	}
}

func TestValidateResultRequiresDiagnosticForUnmatchedClosing(t *testing.T) {
	for _, code := range []diagnostics.Code{diagnostics.P009, diagnostics.P010} {
		t.Run(string(code), func(t *testing.T) {
			result := integrityResult(t)
			result.Lines = []model.Line{
				{Number: 1, LineType: model.LineTypeTag, Raw: "@task t", LineEnding: model.LineEndingLF},
				{Number: 2, LineType: model.LineTypeBlockEnd, Raw: "}", HasErrors: true, Elements: []model.Element{{ElementType: model.ElementTypeBlockClose, Raw: "}", Start: 1, End: 2}}},
			}
			*result.Document.LineCount = 2
			if code == diagnostics.P010 {
				result.Lines[1].Raw = "} text"
			}
			result.Diagnostics = []model.Diagnostic{{ID: "closing", Source: "run-a", DiagnosticCode: code,
				SeverityLevel: diagnostics.SeverityError, DiagnosticScope: diagnostics.ScopeLine,
				Location: &model.Location{Start: model.Position{Line: 2, Column: 1}, End: model.Position{Line: 2, Column: 2}}}}
			assertIntegrityResult(t, result, false)
			parent := 1
			result.Lines[1].ParentLine, result.Lines[1].NestingLevel = &parent, 1
			assertIntegrityResult(t, result, true)
			result.Lines[1].ParentLine, result.Lines[1].NestingLevel = nil, 0
			result.Diagnostics[0].DiagnosticCode = diagnostics.P008
			assertIntegrityResult(t, result, true)
			result.Diagnostics[0].DiagnosticCode = code
			result.Diagnostics[0].Location.Start.Line, result.Diagnostics[0].Location.End.Line = 1, 1
			assertIntegrityResult(t, result, true)
			result.Diagnostics = nil
			assertIntegrityResult(t, result, true)
		})
	}
}

func TestCheckParentLinksRequiresMatchingOpenBlock(t *testing.T) {
	first, second := 1, 2
	lines := []model.Line{
		{Number: 1, LineType: model.LineTypeBlockStart},
		{Number: 2, LineType: model.LineTypeBlockStart, ParentLine: &first, NestingLevel: 1},
		{Number: 3, LineType: model.LineTypeBlockEnd, ParentLine: &second, NestingLevel: 2},
		{Number: 4, LineType: model.LineTypeBlockEnd, ParentLine: &first, NestingLevel: 1},
	}
	if err := checkParentLinks(lines, nil); err != nil {
		t.Fatal(err)
	}
	lines[2].ParentLine, lines[2].NestingLevel = &first, 1
	if err := checkParentLinks(lines, nil); err == nil {
		t.Fatal("Принято закрытие внешнего блока раньше внутреннего")
	}
	lines[2].ParentLine, lines[2].NestingLevel = &second, 2
	lines[3].ParentLine, lines[3].NestingLevel = &second, 2
	if err := checkParentLinks(lines, nil); err == nil {
		t.Fatal("Принято повторное закрытие уже закрытого блока")
	}
}

func TestCheckDiagnosticRangesUsesUnicodeAndExclusiveEnds(t *testing.T) {
	for _, test := range []struct {
		name       string
		start, end model.Position
		wantError  bool
	}{
		{"whole Unicode line", model.Position{Line: 3, Column: 1}, model.Position{Line: 3, Column: 3}, false},
		{"empty Unicode end", model.Position{Line: 3, Column: 3}, model.Position{Line: 3, Column: 3}, false},
		{"empty start", model.Position{Line: 3, Column: 1}, model.Position{Line: 3, Column: 1}, false},
		{"multiline block", model.Position{Line: 2, Column: 7}, model.Position{Line: 3, Column: 2}, false},
		{"byte offset", model.Position{Line: 3, Column: 1}, model.Position{Line: 3, Column: 7}, true},
		{"reversed columns", model.Position{Line: 3, Column: 3}, model.Position{Line: 3, Column: 2}, true},
		{"reversed lines", model.Position{Line: 3, Column: 1}, model.Position{Line: 2, Column: 2}, true},
		{"nonexistent line", model.Position{Line: 5, Column: 1}, model.Position{Line: 5, Column: 1}, true},
		{"line zero", model.Position{Column: 1}, model.Position{Column: 1}, true},
		{"column zero", model.Position{Line: 3}, model.Position{Line: 3, Column: 1}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, related := range []bool{false, true} {
				result := integrityResult(t)
				location := model.Location{Start: test.start, End: test.end}
				diagnostic := model.Diagnostic{ID: "range", DiagnosticScope: diagnostics.ScopeBlock}
				if related {
					diagnostic.RelatedLocations = []model.Location{location}
				} else {
					diagnostic.Location = &location
				}
				result.Diagnostics = []model.Diagnostic{diagnostic}
				if err := checkDiagnosticRanges(result); (err != nil) != test.wantError {
					t.Fatalf("checkDiagnosticRanges(), related=%t: %v, наличие ошибки должно быть %t", related, err, test.wantError)
				}
			}
		})
	}
}

func integrityResult(t *testing.T) *model.Result {
	t.Helper()
	count, parent := 4, 2
	text, heading, title, tag, open, close := "Ё🌍", "1", "A", "text", "{", "}"
	location := model.Location{Start: model.Position{Line: 3, Column: 1}, End: model.Position{Line: 3, Column: 3}}
	secondLocation := location
	return &model.Result{
		Format: model.FormatVersion1, Document: model.Document{LineCount: &count, HasErrors: true},
		Processing: []model.Processing{{ID: "run-a"}, {ID: "run-b"}},
		Lines: []model.Line{
			{Number: 1, LineType: model.LineTypeHeading, Raw: "# A", LineEnding: model.LineEndingLF, Elements: []model.Element{
				{ElementType: model.ElementTypeHeadingLevel, Raw: "#", Value: &heading, Start: 1, End: 2}, {ElementType: model.ElementTypeTitle, Raw: "A", Value: &title, Start: 3, End: 4}}},
			{Number: 2, LineType: model.LineTypeBlockStart, Raw: "@text {", LineEnding: model.LineEndingCRLF, Elements: []model.Element{
				{ElementType: model.ElementTypeTag, Raw: "@text", Value: &tag, Start: 1, End: 6}, {ElementType: model.ElementTypeBlockOpen, Raw: "{", Value: &open, Start: 7, End: 8}}},
			{Number: 3, LineType: model.LineTypeContent, Raw: text, LineEnding: model.LineEndingLF, ParentLine: &parent, NestingLevel: 1, HasErrors: true,
				Elements: []model.Element{{ElementType: model.ElementTypeContent, Raw: text, Value: &text, Start: 1, End: 3, ErrorIDs: []string{"d1", "d2"}}}},
			{Number: 4, LineType: model.LineTypeBlockEnd, Raw: "}", ParentLine: &parent, NestingLevel: 1,
				Elements: []model.Element{{ElementType: model.ElementTypeBlockClose, Raw: "}", Value: &close, Start: 1, End: 2}}},
		},
		Diagnostics: []model.Diagnostic{
			{ID: "d1", Source: "run-a", DiagnosticCode: diagnostics.Code("CUSTOM1"), SeverityLevel: diagnostics.SeverityError,
				DiagnosticScope: diagnostics.ScopeElement, Location: &location, RelatedLocations: []model.Location{
					{Start: model.Position{Line: 2, Column: 7}, End: model.Position{Line: 2, Column: 8}}}},
			{ID: "d2", Source: "run-b", DiagnosticCode: diagnostics.Code("CUSTOM2"), SeverityLevel: diagnostics.SeverityWarning,
				DiagnosticScope: diagnostics.ScopeElement, Location: &secondLocation},
		},
	}
}

func assertIntegrityResult(t *testing.T, result *model.Result, wantError bool) {
	t.Helper()
	before, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateResult(result); (err != nil) != wantError {
		t.Fatalf("validateResult() = %v, наличие ошибки должно быть %t", err, wantError)
	}
	after, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("Проверка изменила модель")
	}
}
