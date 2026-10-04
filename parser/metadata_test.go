package parser

import (
	"slices"
	"testing"

	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestCollectMetadataUsesGrammarValues(t *testing.T) {
	p := inputParser(t, "")
	for i, raw := range []string{
		"@DOCUMENT-ID MiXeD_ID", "@section Student\tBook", "@order 000123456789012345678901234567890",
		"# Название", "## Подзаголовок", "### Шаг", "@header Не название документа", "@step Не подзаголовок",
	} {
		line := metadataLine(t, p, raw, i+1)
		before := parserSnapshot(t, line)
		if err := p.collectMetadata(&line); err != nil {
			t.Fatal(err)
		}
		if parserSnapshot(t, line) != before {
			t.Fatal("Готовая строка изменена")
		}
	}
	metadata := p.result.Document.Metadata
	for _, tc := range []struct {
		got  *string
		want string
	}{
		{metadata.DocumentID, "MiXeD_ID"}, {metadata.Section, "Student\tBook"},
		{metadata.Order, "000123456789012345678901234567890"}, {metadata.Title, "Название"}, {metadata.Subtitle, "Подзаголовок"},
	} {
		if tc.got == nil || *tc.got != tc.want {
			t.Fatalf("Утрачено точное значение %q", tc.want)
		}
	}
}

func TestMetadataSingletonRepeatsRemainAmbiguous(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		field func(*model.DocumentMetadata) **string
	}{
		{"@document-id First", func(m *model.DocumentMetadata) **string { return &m.DocumentID }},
		{"@section First", func(m *model.DocumentMetadata) **string { return &m.Section }},
		{"@order 001", func(m *model.DocumentMetadata) **string { return &m.Order }},
		{"# First", func(m *model.DocumentMetadata) **string { return &m.Title }},
		{"## First", func(m *model.DocumentMetadata) **string { return &m.Subtitle }},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			p := inputParser(t, "")
			for number := 1; number <= 3; number++ {
				line := metadataLine(t, p, tc.raw, number)
				before, history := parserSnapshot(t, line), parserSnapshot(t, p.result.Lines)
				if err := p.collectMetadata(&line); err != nil {
					t.Fatal(err)
				}
				if (*tc.field(&p.result.Document.Metadata) != nil) != (number == 1) {
					t.Fatalf("Неверная однозначность после объявления %d", number)
				}
				if parserSnapshot(t, line) != before || parserSnapshot(t, p.result.Lines) != history || len(p.result.Diagnostics) != 0 {
					t.Fatal("Повтор изменил строки или создал диагностику")
				}
				p.result.Lines = append(p.result.Lines, line)
			}
		})
	}
}

func TestAmbiguousMetadataValuesStayNil(t *testing.T) {
	for _, raw := range []string{"@document-id Original", "@section Original", "@order 01", "# Original", "## Original"} {
		for _, damage := range []string{"missing", "nil", "multiple", "unparsed", "wrong kind"} {
			t.Run(raw+"/"+damage, func(t *testing.T) {
				p := inputParser(t, "")
				line := metadataLine(t, p, raw, 1)
				switch damage {
				case "missing":
					line.Elements = line.Elements[:1]
				case "nil":
					line.Elements[1].Value = nil
				case "multiple":
					line.Elements = append(line.Elements, line.Elements[1])
				case "unparsed":
					line.Elements = append(line.Elements, model.Element{ElementType: model.ElementTypeUnparsed})
				case "wrong kind":
					line.Elements[1].ElementType = model.ElementTypeContent
				}
				if err := p.collectMetadata(&line); err != nil {
					t.Fatal(err)
				}
				m := p.result.Document.Metadata
				if m.DocumentID != nil || m.Section != nil || m.Order != nil || m.Title != nil || m.Subtitle != nil {
					t.Fatal("Неоднозначное объявление дало значение")
				}
				line = metadataLine(t, p, raw, 2)
				if err := p.collectMetadata(&line); err != nil {
					t.Fatal(err)
				}
				m = p.result.Document.Metadata
				if m.DocumentID != nil || m.Section != nil || m.Order != nil || m.Title != nil || m.Subtitle != nil {
					t.Fatal("Повтор после неоднозначного объявления восстановил значение")
				}
			})
		}
	}
}

func TestResourceMetadataPreservesPathOrderAndKnownValues(t *testing.T) {
	p := inputParser(t, "")
	for i, raw := range []string{`@resource-dir Audio, "../Shared resources", Audio`, `@resource-dir "", "  Video  ", C:\Data`, `@resource-dir`} {
		line := metadataLine(t, p, raw, i+1)
		if err := p.collectMetadata(&line); err != nil {
			t.Fatal(err)
		}
	}
	partial := metadataLine(t, p, "@resource-dir Known, Missing", 4)
	partial.Elements[2].Value = nil
	partial.Elements = append(partial.Elements, model.Element{ElementType: model.ElementTypeUnparsed})
	if err := p.collectMetadata(&partial); err != nil {
		t.Fatal(err)
	}
	want := []string{"Audio", "../Shared resources", "Audio", "", "  Video  ", `C:\Data`, "Known"}
	if !slices.Equal(p.result.Document.Metadata.ResourceDirs, want) || len(p.result.Diagnostics) != 0 {
		t.Fatalf("Неверные пути: %q", p.result.Document.Metadata.ResourceDirs)
	}
	if p.seenMetadata["@resource-dir"] != 1 {
		t.Fatal("Утрачено место первого объявления")
	}
}

func TestMetadataIgnoresContentAndUnrecognizedMarkers(t *testing.T) {
	for _, kind := range []model.LineType{model.LineTypeContent, model.LineTypeInvalid, model.LineTypeBlockStart, model.LineTypeBlockEnd, model.LineTypeBlank, model.LineTypeSeparator} {
		p := inputParser(t, "")
		for _, raw := range []string{"@document-id Hidden", "@dsl-version 2.0", "# Hidden", "@resource-dir Hidden"} {
			line := metadataLine(t, p, raw, 1)
			line.LineType = kind
			before := parserSnapshot(t, p.result)
			if err := p.collectMetadata(&line); err != nil {
				t.Fatal(err)
			}
			if parserSnapshot(t, p.result) != before || len(p.seenMetadata) != 0 {
				t.Fatalf("Из типа %s извлечены метаданные", kind)
			}
		}
	}
	for _, damage := range []string{"missing marker", "nil marker", "multiple markers", "unknown marker"} {
		for _, raw := range []string{"@document-id Hidden", "# Hidden"} {
			p := inputParser(t, "")
			line := metadataLine(t, p, raw, 1)
			switch damage {
			case "missing marker":
				line.Elements = line.Elements[1:]
			case "nil marker":
				line.Elements[0].Value = nil
			case "multiple markers":
				line.Elements = append(line.Elements, line.Elements[0])
			case "unknown marker":
				value := "unknown"
				line.Elements[0].Value = &value
			}
			before := parserSnapshot(t, p.result)
			if err := p.collectMetadata(&line); err != nil {
				t.Fatal(err)
			}
			if parserSnapshot(t, p.result) != before || len(p.seenMetadata) != 0 {
				t.Fatal("Объявление восстановлено из Raw")
			}
		}
	}
}

func TestVersionFirstLineIsNotRepeatAndLaterDeclarationsCannotRestoreIt(t *testing.T) {
	p := inputParser(t, "@dsl-version 1.2\nText")
	if _, ready, err := p.prepareInput(); err != nil || !ready {
		t.Fatalf("Подготовка: ready=%t, err=%v", ready, err)
	}
	first := metadataLine(t, p, "@DSL-VERSION 1.2", 1)
	if repeated, err := p.checkMetadataRepeat(&first); err != nil || repeated {
		t.Fatalf("Ложный повтор первой версии: %v", err)
	}
	if err := p.collectMetadata(&first); err != nil {
		t.Fatal(err)
	}
	if p.result.Document.DSLVersion == nil || *p.result.Document.DSLVersion != "1.2" {
		t.Fatal("Исходная версия потеряна")
	}
	for i, raw := range []string{"@dsl-version 2.0", "@dsl-version 1.2"} {
		line := metadataLine(t, p, raw, i+2)
		before := parserSnapshot(t, line)
		if err := p.collectMetadata(&line); err != nil {
			t.Fatal(err)
		}
		if p.result.Document.DSLVersion != nil || p.version != "1.2" || p.seenMetadata["@dsl-version"] != 1 || parserSnapshot(t, line) != before || len(p.result.Diagnostics) != 0 {
			t.Fatal("Повтор версии обработан неверно")
		}
	}
}

func TestMetadataCopiesValuesAndDoesNotRejectKnownValuesWithErrors(t *testing.T) {
	for _, raw := range []string{"@document-id Known", "# Known"} {
		p := inputParser(t, "")
		line := metadataLine(t, p, raw, 1)
		line.HasErrors, line.Elements[1].ErrorIDs = true, []string{"existing"}
		line.Raw = "Raw не используется"
		if err := p.collectMetadata(&line); err != nil {
			t.Fatal(err)
		}
		*line.Elements[1].Value = "Changed"
		value := p.result.Document.Metadata.DocumentID
		if line.LineType == model.LineTypeHeading {
			value = p.result.Document.Metadata.Title
		}
		if value == nil || *value != "Known" {
			t.Fatal("Однозначное значение потеряно или разделяет изменяемую память")
		}
	}
	p := inputParser(t, "")
	if err := p.collectMetadata(nil); err == nil {
		t.Fatal("Принята отсутствующая строка")
	}
	if _, err := p.checkMetadataRepeat(nil); err == nil {
		t.Fatal("Принята отсутствующая строка проверки")
	}
	if err := extractMetadataValue(nil, model.Line{}); err == nil {
		t.Fatal("Приняты отсутствующие метаданные")
	}
}

func metadataLine(t *testing.T, p *Parser, raw string, number int) model.Line {
	t.Helper()
	registered, found := p.grammars.Lookup("1.2")
	if !found {
		t.Fatal("Не зарегистрирована GR12")
	}
	context := grammar.NewContext(raw)
	context.SetLine(model.Line{Number: number, Raw: raw})
	context.SetDiagnosticRegistry(p.diagnostics)
	for _, detect := range registered.DetectionFuncs() {
		if !detect(context) {
			t.Fatal("Не завершена детекция")
		}
	}
	for _, orchestrate := range registered.OrchestrationFuncs() {
		if !orchestrate(context) {
			t.Fatal("Не завершён разбор тестовой строки")
		}
	}
	return context.Line()
}
