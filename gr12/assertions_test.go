package gr12

import (
	"encoding/json"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestAssertionIdentifiers(t *testing.T) {
	var increase, decrease grammar.AssertionID = IncreasesNesting, DecreasesNesting
	if increase != "increases-nesting" || decrease != "decreases-nesting" {
		t.Fatalf("Неверные идентификаторы: %q, %q", increase, decrease)
	}
}

func TestIncreasesNestingFromDSLFormsAndLocalErrors(t *testing.T) {
	for _, test := range []struct {
		source string
		want   bool
	}{
		// Блочные формы из DSL 2.8, 3.2–3.12 и 4.1, 4.4–4.9.
		{"@editor {", true}, {"@instr {", true}, {"@hint {", true}, {"@answer {", true},
		{"@example How to {", true}, {"@wordlist Nationalities {", true}, {"@note Tip {", true}, {"@alt Изображение 4A {", true},
		{"@table Pronoun forms {", true}, {"@script Track 01 — Transcript {", true}, {"@text City Life {", true}, {"@key Answers {", true},
		{"@choice Choose a country. {", true}, {"@multichoice Select your favourite activities. {", true},
		{"@matching Answer each question. {", true}, {"@ordering Put the stages in order. {", true},
		{"@fragment grammar-note {", true}, {"@variants {", true}, {"@multifill Complete the text. {", true},
		{"@task task-a", true}, {" \t@TaSk Ёж🌍 \t", true},
		// Локальные P004/P006/P007/P008 сохраняют однозначное открытие.
		{"@note{", true}, {"@text City Life { Hello", true}, {"@hint Title {", true},
		{"@fragment {", true}, {"@fragment part extra {", true}, {"@variants Title {", true},
		{"@task\ttask-a", true}, {"@task", true}, {"@task task-a extra", true},
		// P005 не разрешает открывать неподдерживаемую форму.
		{"@task task-a {", false}, {" \t@TASK Ёж🌍{", false}, {"@task {", false},
		{"@question {", false}, {"@variant A {", false}, {"@table Data", false},
		{"@step Step 2. Read the text", false}, {"@variant Student A", false}, {"@endtask", false},
		{"@media audio 12", false}, {"@note Text", false}, {"@multifill", false},
		{"@instr underline the text { }", false}, {`@note Use \{ and \} as text.`, false},
		{`\@task task-a`, false}, {"Use {x} here.", false}, {"# Unit 1", false},
		{"{", false}, {"}", false}, {"@unknown {", false}, {"", false},
	} {
		t.Run(test.source, func(t *testing.T) {
			line := assertionTestLine(t, 20, test.source, nil)
			assertPureAssertion(t, increasesNesting, grammar.AssertionInput{Line: line}, test.want)
		})
	}
}

func TestIncreasesNestingRequiresStructuralElements(t *testing.T) {
	for _, test := range []struct {
		name string
		line model.Line
		want bool
	}{
		{"block without opening element", model.Line{LineType: model.LineTypeBlockStart, Raw: "@note {", Elements: []model.Element{operandWant(model.ElementTypeTag, "@note", "note", 1)}}, false},
		{"block without tag", model.Line{LineType: model.LineTypeBlockStart, Elements: []model.Element{operandWant(model.ElementTypeBlockOpen, "{", "{", 1)}}, false},
		{"unsupported block tag", model.Line{LineType: model.LineTypeBlockStart, Elements: []model.Element{operandWant(model.ElementTypeTag, "@task", "task", 1), operandWant(model.ElementTypeBlockOpen, "{", "{", 7)}}, false},
		{"null tag value", model.Line{LineType: model.LineTypeTag, Elements: []model.Element{{ElementType: model.ElementTypeTag, Raw: "@task"}}}, false},
		{"text element is not opening", model.Line{LineType: model.LineTypeBlockStart, Elements: []model.Element{operandWant(model.ElementTypeTag, "@note", "note", 1), operandWant(model.ElementTypeContent, "{", "{", 7)}}, false},
		{"task with structural brace", model.Line{LineType: model.LineTypeTag, Elements: []model.Element{operandWant(model.ElementTypeTag, "@task", "task", 1), operandWant(model.ElementTypeBlockOpen, "{", "{", 7)}}, false},
		{"task from typed elements", model.Line{LineType: model.LineTypeTag, Elements: []model.Element{operandWant(model.ElementTypeTag, "@TASK", "task", 1)}}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertPureAssertion(t, increasesNesting, grammar.AssertionInput{Line: test.line}, test.want)
		})
	}
}

func TestDecreasesNestingAtEveryTaskBoundary(t *testing.T) {
	task := assertionTestLine(t, 10, "@task task-a", nil)
	step := assertionTestLine(t, 11, "@step Step 2. Read the text", &task.Number)
	textBlock := assertionTestLine(t, 12, "@text City Life {", &task.Number)
	for _, test := range []struct {
		source string
		want   bool
	}{
		// DSL 2.3.7: явные построчные границы; EOF сюда не относится.
		{"@task task-b", true}, {"@endtask", true}, {"@header Vocabulary practice", true},
		{"@newpage", true}, {"@variants {", true}, {"### Step 2. Practice", true},
		{"@ENDTASK extra", true}, {"@variants Title {", true},
		{"# Unit 1", false}, {"## Vocabulary", false}, {"#### Not supported", false},
		{"@step Step 2. Read the text", false}, {"@variant Student B", false},
		{"@note Tip {", false}, {"@speaking", false}, {"@answer New York", false},
		{"@taskText task-b", false}, {`\@endtask`, false}, {"Text @endtask", false},
		{"---", false}, {"", false}, {"}", false},
	} {
		t.Run(test.source, func(t *testing.T) {
			line := assertionTestLine(t, 20, test.source, nil)
			// Будущая глубина текущей строки не определяет уровень проверки.
			line.NestingLevel = 123
			assertPureAssertion(t, decreasesNesting, grammar.AssertionInput{Line: line, CandidateParent: &task}, test.want)
			for _, parent := range []*model.Line{nil, &step, &textBlock} {
				assertPureAssertion(t, decreasesNesting, grammar.AssertionInput{Line: line, CandidateParent: parent}, false)
			}
		})
	}
	assertPureAssertion(t, decreasesNesting, grammar.AssertionInput{CandidateParent: &task}, false)
}

func TestDecreasesNestingClosesOpenBlockRegardlessOfCandidate(t *testing.T) {
	variants := assertionTestLine(t, 3, "@variants {", nil)
	task := assertionTestLine(t, 10, "@task a-1", &variants.Number)
	child := assertionTestLine(t, 11, "@text City Life {", &task.Number)
	step := assertionTestLine(t, 12, "@step Step 2. Read the text", &task.Number)
	for _, source := range []string{"}", " \t} \t", "} @note Text", "} }"} {
		t.Run(source, func(t *testing.T) {
			line := assertionTestLine(t, 20, source, nil)
			for _, parent := range []*model.Line{nil, &task, &step, &child, &variants} {
				assertPureAssertion(t, decreasesNesting, grammar.AssertionInput{Line: line, CandidateParent: parent}, false)
				// Уточнение пользователя: true относится к OpenBlock, независимо от кандидата.
				// Дочерний text закрывается без изменения task; variants закрывает контейнер.
				for _, open := range []*model.Line{&child, &variants} {
					assertPureAssertion(t, decreasesNesting, grammar.AssertionInput{Line: line, OpenBlock: open, CandidateParent: parent}, true)
				}
			}
		})
	}
	for _, source := range []string{`\}`, "Text }", "", "{", "---"} {
		line := assertionTestLine(t, 20, source, nil)
		assertPureAssertion(t, decreasesNesting, grammar.AssertionInput{Line: line, OpenBlock: &child, CandidateParent: &task}, false)
	}
	assertPureAssertion(t, decreasesNesting, grammar.AssertionInput{Line: model.Line{LineType: model.LineTypeBlockEnd, Raw: "}"}, OpenBlock: &child, CandidateParent: &task}, false)
}

func TestDecreasesNestingChangesVariantOnlyInContainingVariants(t *testing.T) {
	variants := assertionTestLine(t, 3, "@variants {", nil)
	task := assertionTestLine(t, 10, "@task a-1", &variants.Number)
	rootTask := assertionTestLine(t, 10, "@task task-a", nil)
	unrelated := assertionTestLine(t, 4, "@variants {", nil)
	child := assertionTestLine(t, 11, "@text City Life {", &task.Number)
	note := assertionTestLine(t, 3, "@note Tip {", nil)
	wrongType := variants
	wrongType.LineType = model.LineTypeTag
	line := assertionTestLine(t, 20, "@variant Student B", nil)
	for _, test := range []struct {
		name         string
		open, parent *model.Line
		want         bool
	}{
		{"containing variants", &variants, &task, true},
		{"no open block", nil, &task, false}, {"task outside variants", &variants, &rootTask, false},
		{"different variants", &unrelated, &task, false}, {"child block", &child, &task, false},
		{"different container tag", &note, &task, false}, {"container without block type", &wrongType, &task, false},
		{"candidate is variants", &variants, &variants, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertPureAssertion(t, decreasesNesting, grammar.AssertionInput{Line: line, OpenBlock: test.open, CandidateParent: test.parent}, test.want)
		})
	}
}

func TestAssertionsPreserveAliasedElementsAndErrorReferences(t *testing.T) {
	line := assertionTestLine(t, 3, "@variants Title {", nil)
	references := []string{"existing-error"}
	for i := range line.Elements {
		line.Elements[i].ErrorIDs = references
	}
	input := grammar.AssertionInput{Line: line, OpenBlock: &line, CandidateParent: &line}
	assertPureAssertion(t, increasesNesting, input, true)
	assertPureAssertion(t, decreasesNesting, input, false)
	if input.OpenBlock != &line || input.CandidateParent != &line || references[0] != "existing-error" {
		t.Fatal("Изменены общие ссылки входа")
	}
	for _, source := range []*model.Line{&input.Line, input.OpenBlock, input.CandidateParent} {
		for i := range source.Elements {
			if len(source.Elements[i].ErrorIDs) != 1 || &source.Elements[i].ErrorIDs[0] != &references[0] {
				t.Fatal("Заменено общее хранилище ссылок на диагностики")
			}
		}
	}
}

// assertionTestLine получает реальные элементы из примера DSL без состояния парсера.
func assertionTestLine(t *testing.T, number int, source string, parent *int) model.Line {
	t.Helper()
	context := grammar.NewContext(source)
	context.SetDiagnosticRegistry(diagnostics.NewRegistry())
	context.SetLine(model.Line{Number: number, Raw: source, ParentLine: parent, LineEnding: model.LineEndingCRLF})
	if !classifyLine(context) {
		t.Fatal("Не удалось классифицировать пример")
	}
	parseLine(context)
	line := context.Line()
	line.HasErrors = len(context.Diagnostics()) != 0
	if parent != nil {
		line.NestingLevel = 2
	}
	return line
}

func assertPureAssertion(t *testing.T, check grammar.AssertionFunc, input grammar.AssertionInput, want bool) {
	t.Helper()
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	lines := []*model.Line{&input.Line, input.OpenBlock, input.CandidateParent}
	parents := make([]*int, len(lines))
	values := make([][]*string, len(lines))
	elements := make([][]*model.Element, len(lines))
	for i, line := range lines {
		if line != nil {
			parents[i] = line.ParentLine
			for j := range line.Elements {
				values[i] = append(values[i], line.Elements[j].Value)
				elements[i] = append(elements[i], &line.Elements[j])
			}
		}
	}
	for repeat := 0; repeat < 3; repeat++ {
		if got := check(input); got != want {
			t.Errorf("Утверждение вернуло %t, требуется %t: %s", got, want, before)
		}
		// Чередование с пустым входом проверяет отсутствие памяти между вызовами.
		if increasesNesting(grammar.AssertionInput{}) || decreasesNesting(grammar.AssertionInput{}) {
			t.Fatal("Ответ зависит от предыдущего вызова")
		}
	}
	after, err := json.Marshal(input)
	if err != nil || string(after) != string(before) {
		t.Fatalf("Изменены входные данные: до %s, после %s, ошибка %v", before, after, err)
	}
	for i, line := range lines {
		if line != nil {
			if line.ParentLine != parents[i] {
				t.Fatal("Изменён указатель на родителя")
			}
			for j := range line.Elements {
				if &line.Elements[j] != elements[i][j] || line.Elements[j].Value != values[i][j] {
					t.Fatal("Изменено хранилище элементов или указатель на значение")
				}
			}
		}
	}
}
