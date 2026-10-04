package parser

import (
	"slices"
	"strings"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestNestingParentsAndBlockDepth(t *testing.T) {
	for _, tc := range []struct {
		name            string
		raw             []string
		parents, depths []int
	}{
		{"sibling tasks", []string{"@task A", "Text", "@task B", "Text"}, []int{0, 1, 0, 3}, []int{0, 1, 0, 1}},
		{"child block", []string{"@task A", "@note {", "Text", "}", "Text", "@endtask"}, []int{0, 1, 2, 2, 1, 0}, []int{0, 1, 2, 2, 1, 0}},
		{"nested blocks", []string{"@variants {", "@note {", "Text", "}", "}", "Text"}, []int{0, 1, 2, 2, 1, 0}, []int{0, 1, 2, 2, 1, 0}},
		{"container closes inner task", []string{"@variants {", "@task A", "}", "Text"}, []int{0, 1, 1, 0}, []int{0, 1, 1, 0}},
		{"variant ends contained task", []string{"@variants {", "@task A", "@variant B", "Text", "}"}, []int{0, 1, 1, 1, 1}, []int{0, 1, 1, 1, 1}},
		{"variant outside container", []string{"@task A", "@variant B", "Text"}, []int{0, 1, 1}, []int{0, 1, 1}},
		{"root headings", []string{"@task A", "# Title", "## Subtitle", "Text", "### Step", "Text"}, []int{0, 0, 0, 1, 0, 0}, []int{0, 0, 0, 1, 0, 0}},
		{"local errors", []string{"@task", "@fragment {", "Text", "} extra", "@unknown", "Text"}, []int{0, 1, 2, 2, 1, 1}, []int{0, 1, 2, 2, 1, 1}},
		{"opening with tail error", []string{"@task A", "@text Title { extra", "Text", "}", "Text"}, []int{0, 1, 2, 2, 1}, []int{0, 1, 2, 2, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := nestingParser(t)
			for i, raw := range tc.raw {
				line := nestingAdd(t, p, raw)
				parent := 0
				if line.ParentLine != nil {
					parent = *line.ParentLine
				}
				if parent != tc.parents[i] || line.NestingLevel != tc.depths[i] {
					t.Fatalf("Строка %d: родитель %d, глубина %d", line.Number, parent, line.NestingLevel)
				}
			}
			if err := checkParentLinks(p.result.Lines); err != nil {
				t.Fatal(err)
			}
			for _, diagnostic := range p.result.Diagnostics {
				if diagnostic.Fatal {
					t.Fatalf("Локальная ошибка стала фатальной: %+v", diagnostic)
				}
			}
		})
	}
}

func TestTaskEndsAtGrammarBoundaries(t *testing.T) {
	for _, boundary := range []string{"@endtask", "@header Header", "@newpage", "### Step", "@variants {"} {
		t.Run(boundary, func(t *testing.T) {
			p := nestingParser(t)
			nestingAdd(t, p, "@task A")
			line := nestingAdd(t, p, boundary)
			if line.ParentLine != nil || line.NestingLevel != 0 || len(p.parents) != 0 {
				t.Fatal("Предыдущее задание не завершено до выбора родителя")
			}
			next := nestingAdd(t, p, "Text")
			if line.LineType == model.LineTypeBlockStart {
				if next.ParentLine == nil || *next.ParentLine != line.Number {
					t.Fatal("Не выбран новый блок")
				}
			} else if next.ParentLine != nil {
				t.Fatal("Следующая строка сохранила прежнее задание")
			}
		})
	}
}

func TestOrphanClosingDoesNotCloseTask(t *testing.T) {
	for _, raw := range []string{"}", " \t} \t", "} extra"} {
		t.Run(raw, func(t *testing.T) {
			p := nestingParser(t)
			nestingAdd(t, p, "@task A")
			closing := nestingAdd(t, p, raw)
			if closing.ParentLine != nil || closing.NestingLevel != 0 || !slices.Equal(p.parents, []int{1}) || len(p.openBlocks) != 0 {
				t.Fatal("Логическое задание принято за фигурный блок")
			}
			want := diagnostics.P009
			if raw == "} extra" {
				want = diagnostics.P010
			}
			if len(p.result.Diagnostics) != 1 {
				t.Fatalf("Лишние диагностики: %+v", p.result.Diagnostics)
			}
			diagnostic := p.result.Diagnostics[0]
			if diagnostic.DiagnosticCode != want || diagnostic.Fatal || diagnostic.DiagnosticScope != diagnostics.ScopeLine || diagnostic.Location == nil || diagnostic.Location.Start.Line != 2 || diagnostic.Source != "current" {
				t.Fatalf("Неверное закрытие: %+v", diagnostic)
			}
			if err := p.closeBlock(closing); err != nil || len(p.result.Diagnostics) != 1 {
				t.Fatal("Повтор диагностики закрытия")
			}
			if next := nestingAdd(t, p, "Text"); next.ParentLine == nil || *next.ParentLine != 1 {
				t.Fatal("Задание не сохранено")
			}
			if err := p.checkOpenBlocks(); err != nil || len(p.result.Diagnostics) != 1 {
				t.Fatal("Задание потребовало фигурного закрытия на EOF")
			}
		})
	}
}

func TestUnclosedBlocksReportOpeningAndKeepLines(t *testing.T) {
	p := nestingParser(t)
	nestingAdd(t, p, "@variants {")
	nestingAdd(t, p, "@note Заголовок {")
	nestingAdd(t, p, "Text")
	before := parserSnapshot(t, p.result.Lines)
	if err := p.checkOpenBlocks(); err != nil {
		t.Fatal(err)
	}
	if parserSnapshot(t, p.result.Lines) != before || len(p.result.Diagnostics) != 2 {
		t.Fatal("Потеряны строки или незакрытые блоки")
	}
	for i, tag := range []string{"@variants", "@note"} {
		diagnostic := p.result.Diagnostics[i]
		opening := p.result.Lines[i]
		brace := opening.Elements[len(opening.Elements)-1]
		if diagnostic.DiagnosticCode != diagnostics.P011 || !diagnostic.Fatal || diagnostic.SeverityLevel != diagnostics.SeverityError || diagnostic.DiagnosticScope != diagnostics.ScopeBlock || diagnostic.Source != "current" || !strings.Contains(diagnostic.Message, tag) {
			t.Fatalf("Неверная P011: %+v", diagnostic)
		}
		want := model.Location{Start: model.Position{Line: opening.Number, Column: brace.Start}, End: model.Position{Line: opening.Number, Column: brace.End}}
		if diagnostic.Location == nil || *diagnostic.Location != want {
			t.Fatal("Потеряно место открытия")
		}
	}
	if err := checkDiagnosticRanges(p.result); err != nil {
		t.Fatal(err)
	}
}

func TestResetNestingKeepsSavedLinesAndResumesAtRoot(t *testing.T) {
	p := nestingParser(t)
	nestingAdd(t, p, "@task A")
	nestingAdd(t, p, "@note {")
	before := parserSnapshot(t, p.result.Lines)
	parent := 2
	broken := model.Line{Number: 3, Raw: "broken", ParentLine: &parent, NestingLevel: 2, LineType: model.LineTypeInvalid}
	p.resetNesting(&broken)
	if len(p.parents) != 0 || len(p.openBlocks) != 0 || broken.ParentLine != nil || broken.NestingLevel != 0 || parserSnapshot(t, p.result.Lines) != before {
		t.Fatal("Сброс изменил сохранённые строки или оставил ненадёжное состояние")
	}
	p.result.Lines = append(p.result.Lines, broken)
	if line := nestingAdd(t, p, "Text"); line.ParentLine != nil || line.NestingLevel != 0 {
		t.Fatal("Сброс не возобновил разбор от корня")
	}
	if err := p.checkOpenBlocks(); err != nil || len(p.result.Diagnostics) != 0 {
		t.Fatal("Сброшенный блок остался активным")
	}
}

func TestNestingUsesRoleIDsAndQueriesBeforeAndAfterSaving(t *testing.T) {
	p := nestingParser(t)
	p.enterParentID, p.leaveParentID = "custom-enter", "custom-leave"
	var calls []string
	p.assertions = map[grammar.AssertionID]grammar.AssertionFunc{
		p.leaveParentID: func(input grammar.AssertionInput) bool {
			if input.CandidateParent == nil || input.CandidateParent.Number != 1 || input.Line.Number != 2 || input.Line.ParentLine != nil {
				t.Fatal("Неверные данные запроса выхода")
			}
			if len(p.result.Lines) == 1 {
				calls = append(calls, "preview")
			} else {
				calls = append(calls, "leave")
			}
			return true
		},
		p.enterParentID: func(input grammar.AssertionInput) bool { calls = append(calls, "enter"); return true },
	}
	p.result.Lines = []model.Line{{Number: 1, LineType: model.LineTypeTag}}
	p.parents = []int{1}
	line := model.Line{Number: 2, LineType: model.LineTypeTag, Raw: "не DSL, решение даёт утверждение"}
	if err := p.assignParent(&line); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.parents, []int{1}) || line.ParentLine != nil {
		t.Fatal("Предварительный запрос изменил стек")
	}
	context := grammar.NewContext(line.Raw)
	context.SetAssertions([]grammar.Assertion{{ID: p.enterParentID, Check: p.assertions[p.enterParentID]}, {ID: p.leaveParentID, Check: p.assertions[p.leaveParentID]}})
	p.result.Lines = append(p.result.Lines, line)
	if err := p.updateParents(context, line); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []string{"preview", "leave", "enter"}) || !slices.Equal(p.parents, []int{2}) {
		t.Fatalf("Неверные переходы: %v, %v", calls, p.parents)
	}
}

func TestNestingRejectsBrokenReferences(t *testing.T) {
	for _, block := range []bool{false, true} {
		p := nestingParser(t)
		if block {
			p.openBlocks = []int{9}
		} else {
			p.parents = []int{9}
		}
		line := model.Line{Number: 1}
		if err := p.assignParent(&line); err == nil {
			t.Fatal("Принят отсутствующий родитель")
		}
	}
	p := nestingParser(t)
	if err := p.updateParents(grammar.NewContext(""), model.Line{Number: 1}); err == nil {
		t.Fatal("Стек обновлён до сохранения строки")
	}
	p.openBlocks = []int{1}
	if err := p.checkOpenBlocks(); err == nil {
		t.Fatal("Отсутствующее открытие стало DSL-диагностикой")
	}
}

func nestingParser(t *testing.T) *Parser {
	t.Helper()
	p := inputParser(t, "")
	p.version = "1.2"
	if ok, err := p.selectGrammar(); err != nil || !ok {
		t.Fatalf("Выбор грамматики: %v", err)
	}
	return p
}

func nestingAdd(t *testing.T, p *Parser, raw string) model.Line {
	t.Helper()
	context := grammar.NewContext(raw)
	context.SetDiagnosticRegistry(p.diagnostics)
	context.SetLine(model.Line{Number: len(p.result.Lines) + 1, Raw: raw})
	context.SetAssertions([]grammar.Assertion{{ID: p.enterParentID, Check: p.assertions[p.enterParentID]}, {ID: p.leaveParentID, Check: p.assertions[p.leaveParentID]}})
	for _, detect := range p.detections {
		if !detect(context) {
			t.Fatal("Детекция не завершена")
		}
	}
	for _, orchestrate := range p.orchestrators {
		if !orchestrate(context) {
			t.Fatalf("Разбор %q не завершён", raw)
		}
	}
	line := context.Line()
	parents, blocks := slices.Clone(p.parents), slices.Clone(p.openBlocks)
	if err := p.assignParent(&line); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(parents, p.parents) || !slices.Equal(blocks, p.openBlocks) {
		t.Fatal("Назначение родителя изменило стеки")
	}
	if err := p.importLineDiagnostics(context, &line); err != nil {
		t.Fatal(err)
	}
	p.result.Lines = append(p.result.Lines, line)
	if err := p.updateParents(context, line); err != nil {
		t.Fatal(err)
	}
	return line
}
