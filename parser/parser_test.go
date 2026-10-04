package parser

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/gr12"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestNewCopiesBothAssertionMapLevelsWithoutReadingOrChangingResult(t *testing.T) {
	result := parserInitialResult(t)
	reader := &parserCountingReader{source: strings.NewReader("@dsl-version 1.2\n@task a\nText\n@endtask")}
	grammars := grammar.NewRegistry()
	checks := gr12.BuiltinGr12(grammars)
	assertions := map[string]map[grammar.AssertionID]grammar.AssertionFunc{"1.2": checks}
	before := parserSnapshot(t, result)
	p := New(reader, result, grammars, diagnostics.NewRegistry(), assertions)
	if reader.reads != 0 || parserSnapshot(t, result) != before {
		t.Fatal("Конструктор прочитал поток или изменил модель")
	}
	checks[gr12.IncreasesNesting] = nil
	delete(checks, gr12.DecreasesNesting)
	assertions["1.2"] = nil
	delete(assertions, "1.2")
	if err := p.Parse(); err != nil {
		t.Fatal(err)
	}
	if reader.reads == 0 || len(result.Lines) != 4 {
		t.Fatal("Парсер не использовал закреплённые поток и модель")
	}
	if result.Lines[2].ParentLine == nil || *result.Lines[2].ParentLine != 2 || result.Lines[2].NestingLevel != 1 || result.Lines[3].ParentLine != nil {
		t.Fatal("Изменение исходных карт повредило структурные утверждения")
	}
}

func TestPrepareResultInitializesSlicesAndPreservesExistingData(t *testing.T) {
	result := parserInitialResult(t)
	fileName, title := "lesson.dsl", "Provided title"
	result.Document.FileName, result.Document.Metadata.Title = &fileName, &title
	result.Processing = append([]model.Processing{{ID: "old", Tool: "previous"}}, result.Processing...)
	result.Diagnostics = []model.Diagnostic{{ID: "old", Source: "old", DiagnosticCode: "CUSTOM", Message: "Existing message",
		SeverityLevel: diagnostics.SeverityWarning, DiagnosticScope: diagnostics.ScopeDocument}}
	history := parserSnapshot(t, result.Processing)
	wantDiagnostic := result.Diagnostics[0]
	wantDiagnostic.RelatedLocations = []model.Location{}
	p := New(nil, result, nil, nil, nil)
	if err := p.prepareResult(); err != nil {
		t.Fatal(err)
	}
	if result.Lines == nil || len(result.Lines) != 0 || result.Document.Metadata.ResourceDirs == nil || result.Diagnostics[0].RelatedLocations == nil {
		t.Fatal("Обязательные пустые срезы не подготовлены")
	}
	if parserSnapshot(t, result.Processing) != history || len(result.Diagnostics) != 1 || parserSnapshot(t, result.Diagnostics[0]) != parserSnapshot(t, wantDiagnostic) {
		t.Fatal("Подготовка изменила историю или прежнюю диагностику")
	}
	if result.Document.FileName != &fileName || result.Document.Metadata.Title != &title || p.processingID != "current" {
		t.Fatal("Утрачены сведения документа или выбран неверный запуск")
	}
	empty := parserInitialResult(t)
	if err := New(nil, empty, nil, nil, nil).prepareResult(); err != nil || empty.Diagnostics == nil {
		t.Fatalf("Пустой массив Diagnostics не подготовлен: %v", err)
	}
}

func TestParseRejectsInvalidModelBeforeReadingAndCannotBeRetried(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*model.Result)
	}{
		{"empty format", func(r *model.Result) { r.Format = "" }},
		{"unsupported format", func(r *model.Result) { r.Format = "2.0" }},
		{"existing lines", func(r *model.Result) { r.Lines = []model.Line{{Number: 1}} }},
		{"no processing", func(r *model.Result) { r.Processing = nil }},
		{"empty current ID", func(r *model.Result) { r.Processing[0].ID = "" }},
		{"duplicate current ID", func(r *model.Result) { r.Processing = append(r.Processing, r.Processing[0]) }},
		{"empty previous ID", func(r *model.Result) { r.Processing = append([]model.Processing{{}}, r.Processing...) }},
		{"duplicate previous IDs", func(r *model.Result) {
			r.Processing = append([]model.Processing{{ID: "old"}, {ID: "old"}}, r.Processing...)
		}},
		{"empty diagnostic ID", func(r *model.Result) { r.Diagnostics = []model.Diagnostic{{Source: "current"}} }},
		{"duplicate diagnostic IDs", func(r *model.Result) {
			r.Diagnostics = []model.Diagnostic{{ID: "d", Source: "current"}, {ID: "d", Source: "current"}}
		}},
		{"unknown source", func(r *model.Result) { r.Diagnostics = []model.Diagnostic{{ID: "d", Source: "missing"}} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := parserInitialResult(t)
			test.change(result)
			reader := &parserCountingReader{source: strings.NewReader("@dsl-version 1.2")}
			grammars := grammar.NewRegistry()
			checks := gr12.BuiltinGr12(grammars)
			p := New(reader, result, grammars, diagnostics.NewRegistry(), map[string]map[grammar.AssertionID]grammar.AssertionFunc{"1.2": checks})
			before := parserSnapshot(t, result)
			if err := p.Parse(); err == nil || reader.reads != 0 || parserSnapshot(t, result) != before {
				t.Fatalf("Некорректная модель не отклонена до обработки: %v, чтений %d", err, reader.reads)
			}
			*result = *parserInitialResult(t)
			before = parserSnapshot(t, result)
			if err := p.Parse(); err == nil || reader.reads != 0 || parserSnapshot(t, result) != before {
				t.Fatalf("Исправление модели разрешило повторный запуск: %v, чтений %d", err, reader.reads)
			}
		})
	}
}

func TestParseRejectsMissingInputs(t *testing.T) {
	var absent *Parser
	if err := absent.Parse(); err == nil {
		t.Fatal("Отсутствующий парсер принят")
	}
	for _, missing := range []string{"result", "reader", "registry"} {
		t.Run(missing, func(t *testing.T) {
			reader := &parserCountingReader{source: strings.NewReader("@dsl-version 1.2")}
			var input io.Reader = reader
			result, registry := parserInitialResult(t), diagnostics.NewRegistry()
			switch missing {
			case "result":
				result = nil
			case "reader":
				input = nil
			case "registry":
				registry = nil
			}
			if err := New(input, result, nil, registry, nil).Parse(); err == nil || reader.reads != 0 {
				t.Fatalf("Отсутствующее значение не отклонено до чтения: %v", err)
			}
		})
	}
}

func TestCurrentProcessingIDUsesLastEntry(t *testing.T) {
	for _, test := range []struct {
		name      string
		result    *model.Result
		want      string
		wantError bool
	}{
		{"nil result", nil, "", true},
		{"no history", &model.Result{}, "", true},
		{"empty last ID", &model.Result{Processing: []model.Processing{{ID: "old"}, {}}}, "", true},
		{"repeated last ID", &model.Result{Processing: []model.Processing{{ID: "same"}, {ID: "same"}}}, "", true},
		{"last by position", &model.Result{Processing: []model.Processing{{ID: "z"}, {ID: "a"}}}, "a", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			id, err := currentProcessingID(test.result)
			if (err != nil) != test.wantError || id != test.want {
				t.Fatalf("currentProcessingID() = (%q, %v), требуется ID %q, наличие ошибки %t", id, err, test.want, test.wantError)
			}
		})
	}
}

func TestParseCompletesOnceForSuccessAndInputDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name, source string
		readError    error
		code         diagnostics.Code
		lineCount    int
	}{
		{name: "success", source: "@dsl-version 1.2\r\n# Заголовок\n", lineCount: 2},
		{name: "missing version", source: "Text", code: diagnostics.P013},
		{name: "empty document", code: diagnostics.P013},
		{name: "invalid UTF8", source: "\xff", code: diagnostics.P001},
		{name: "read error", readError: errors.New("read failed"), code: diagnostics.IO001},
		{name: "unclosed block", source: "@dsl-version 1.2\n@text {", code: diagnostics.P011, lineCount: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := parserInitialResult(t)
			reader := &parserCountingReader{source: strings.NewReader(test.source), err: test.readError}
			grammars := grammar.NewRegistry()
			checks := gr12.BuiltinGr12(grammars)
			p := New(reader, result, grammars, diagnostics.NewRegistry(), map[string]map[grammar.AssertionID]grammar.AssertionFunc{"1.2": checks})
			if err := p.Parse(); err != nil {
				t.Fatal(err)
			}
			if reader.reads == 0 || result.Lines == nil || len(result.Lines) != test.lineCount || result.Diagnostics == nil {
				t.Fatal("Не выполнено чтение или неверно подготовлена модель")
			}
			if test.lineCount == 0 {
				if result.Document.LineCount != nil {
					t.Fatal("При раннем отказе LineCount должен быть nil")
				}
			} else if result.Document.LineCount == nil || *result.Document.LineCount != test.lineCount {
				t.Fatal("LineCount не соответствует сохранённым строкам")
			}
			if test.code == "" {
				if len(result.Diagnostics) != 0 || result.Document.HasErrors || result.Lines[0].Raw != "@dsl-version 1.2" || result.Lines[0].LineEnding != model.LineEndingCRLF {
					t.Fatal("Успешный документ изменён или получил лишнюю диагностику")
				}
			} else if len(result.Diagnostics) != 1 || result.Diagnostics[0].DiagnosticCode != test.code || result.Diagnostics[0].Source != "current" || result.Diagnostics[0].ID == "" || !result.Diagnostics[0].Fatal || !result.Document.HasErrors {
				t.Fatalf("Не сохранена ожидаемая диагностика %s: %+v", test.code, result.Diagnostics)
			}
			before, reads := parserSnapshot(t, result), reader.reads
			if err := p.Parse(); err == nil || reader.reads != reads || parserSnapshot(t, result) != before {
				t.Fatalf("Повторный вызов не отклонён без побочных эффектов: %v", err)
			}
		})
	}
}

func TestParsePreservesHistoryAndUsesCurrentSource(t *testing.T) {
	result := parserInitialResult(t)
	result.Processing = append([]model.Processing{{ID: "old", Tool: "previous"}}, result.Processing...)
	result.Diagnostics = []model.Diagnostic{{ID: "d1", Source: "old", DiagnosticCode: "CUSTOM", Message: "Keep this",
		SeverityLevel: diagnostics.SeverityWarning, DiagnosticScope: diagnostics.ScopeDocument, RelatedLocations: []model.Location{}}}
	history, previous := parserSnapshot(t, result.Processing), parserSnapshot(t, result.Diagnostics[0])
	grammars := grammar.NewRegistry()
	checks := gr12.BuiltinGr12(grammars)
	p := New(strings.NewReader("@dsl-version 1.2\n@unknown"), result, grammars, diagnostics.NewRegistry(), map[string]map[grammar.AssertionID]grammar.AssertionFunc{"1.2": checks})
	if err := p.Parse(); err != nil {
		t.Fatal(err)
	}
	if parserSnapshot(t, result.Processing) != history || len(result.Diagnostics) != 2 || parserSnapshot(t, result.Diagnostics[0]) != previous {
		t.Fatal("Утрачены история или прежняя диагностика")
	}
	added := result.Diagnostics[1]
	if added.Source != "current" || added.ID == "" || added.ID == "d1" || added.DiagnosticCode != diagnostics.P003 || added.Fatal || !result.Document.HasErrors {
		t.Fatalf("Неверная новая диагностика: %+v", added)
	}
}

type parserCountingReader struct {
	source io.Reader
	reads  int
	err    error
}

func (r *parserCountingReader) Read(buffer []byte) (int, error) {
	r.reads++
	if r.err != nil {
		return 0, r.err
	}
	return r.source.Read(buffer)
}

func parserInitialResult(t *testing.T) *model.Result {
	t.Helper()
	return &model.Result{Format: model.FormatVersion1, Processing: []model.Processing{{ID: "current", Tool: "dsl-parser"}}}
}

func parserSnapshot(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
