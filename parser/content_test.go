package parser

import (
	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
	"testing"
)

func TestContentBraceDiagnosticsHaveElementLinksAndParsingContinues(t *testing.T) {
	for _, opening := range []string{"", "@text {\n", "@multifill {\n", "@editor {\n"} {
		t.Run(opening, func(t *testing.T) {
			source := "@dsl-version 1.2\n" + opening + "  Ёж🌍 {x}  \n"
			contentIndex := 1
			if opening != "" {
				source += "}\n"
				contentIndex = 2
			}
			source += "@document-id Next\nText"
			p := inputParser(t, source)
			if err := p.Parse(); err != nil {
				t.Fatal(err)
			}
			if len(p.result.Diagnostics) != 1 {
				t.Fatalf("Диагностики: %+v", p.result.Diagnostics)
			}
			d := p.result.Diagnostics[0]
			line := p.result.Lines[contentIndex]
			if d.DiagnosticCode != diagnostics.P012 || d.Fatal || !p.result.Document.HasErrors || !line.HasErrors || len(line.Elements) != 1 {
				t.Fatalf("Неверная P012: %+v; строка %+v", d, line)
			}
			e := line.Elements[0]
			if len(e.ErrorIDs) != 1 || e.ErrorIDs[0] != d.ID || d.Location == nil || d.Location.Start.Line != line.Number || d.Location.Start.Column != e.Start || d.Location.End.Column != e.End {
				t.Fatal("Нарушена обратная ссылка P012")
			}
			if p.result.Document.Metadata.DocumentID == nil || *p.result.Document.Metadata.DocumentID != "Next" {
				t.Fatal("Разбор остановился после локальной ошибки")
			}
		})
	}
}

func TestPlaceholderSyntaxDoesNotValidatePlacement(t *testing.T) {
	for _, opening := range []string{"", "@multifill {\n", "@text {\n", "@editor {\n"} {
		source := "@dsl-version 1.2\n" + opening + `She _____{lives} and \{writes\}.` + "\n"
		if opening != "" {
			source += "}\n"
		}
		p := inputParser(t, source)
		if err := p.Parse(); err != nil {
			t.Fatal(err)
		}
		if len(p.result.Diagnostics) != 0 {
			t.Fatalf("Грамматика проверила размещение вместо синтаксиса: %+v", p.result.Diagnostics)
		}
	}
}

func TestMultifillEmptyLinesRemainBlank(t *testing.T) {
	p := inputParser(t, "@dsl-version 1.2\n@multifill {\n\n \t\n}")
	if err := p.Parse(); err != nil {
		t.Fatal(err)
	}
	for _, line := range p.result.Lines[2:4] {
		if line.LineType != model.LineTypeBlank || line.ParentLine == nil || *line.ParentLine != 2 || line.Elements == nil || len(line.Elements) != 0 {
			t.Fatalf("Пустая строка должна оставаться blank: %+v", line)
		}
	}
	if len(p.result.Diagnostics) != 0 {
		t.Fatalf("Лишние диагностики: %+v", p.result.Diagnostics)
	}
}
