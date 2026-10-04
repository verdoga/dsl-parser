package app

import (
	"strings"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/gr12"
	"github.com/verdoga/dsl-parser/model"
	"github.com/verdoga/dsl-parser/parser"
)

func TestPrepareGrammarRegistersSupportedVersionAndChecks(t *testing.T) {
	grammars, assertions, err := prepareGrammar()
	if err != nil {
		t.Fatal(err)
	}
	registered, found := grammars.Lookup("1.2")
	if !found || registered.Version() != "1.2" {
		t.Fatal("Грамматика DSL 1.2 не зарегистрирована")
	}
	enter, leave := registered.Assertions()
	if enter != gr12.IncreasesNesting || leave != gr12.DecreasesNesting {
		t.Fatalf("Неверные структурные роли: %q, %q", enter, leave)
	}
	if assertions["1.2"][enter] == nil || assertions["1.2"][leave] == nil {
		t.Fatal("Не предоставлены обе функции структурных проверок")
	}
	if _, found := grammars.Lookup("2.0"); found {
		t.Fatal("Неподдерживаемая версия зарегистрирована")
	}
}

func TestPrepareGrammarReturnsIndependentRegistriesAndAssertionMaps(t *testing.T) {
	firstGrammars, firstAssertions, err := prepareGrammar()
	if err != nil {
		t.Fatal(err)
	}
	secondGrammars, secondAssertions, err := prepareGrammar()
	if err != nil {
		t.Fatal(err)
	}
	delete(firstGrammars, "1.2")
	firstAssertions["1.2"][gr12.IncreasesNesting] = nil
	delete(firstAssertions["1.2"], gr12.DecreasesNesting)
	delete(firstAssertions, "1.2")

	if _, found := secondGrammars.Lookup("1.2"); !found {
		t.Fatal("Изменение первого реестра затронуло второй")
	}
	if secondAssertions["1.2"][gr12.IncreasesNesting] == nil || secondAssertions["1.2"][gr12.DecreasesNesting] == nil {
		t.Fatal("Изменение карт первого результата затронуло второй")
	}
	thirdGrammars, thirdAssertions, err := prepareGrammar()
	if err != nil {
		t.Fatal(err)
	}
	if _, found := thirdGrammars.Lookup("1.2"); !found {
		t.Fatal("Изменение прежнего реестра повредило новую подготовку")
	}
	if thirdAssertions["1.2"][gr12.IncreasesNesting] == nil || thirdAssertions["1.2"][gr12.DecreasesNesting] == nil {
		t.Fatal("Изменение прежних карт повредило новую подготовку")
	}
}

func TestPrepareGrammarSupportsParserNestingAndUnknownVersion(t *testing.T) {
	for _, test := range []struct {
		name    string
		source  string
		version string
		parents []int
		depths  []int
		code    diagnostics.Code
	}{
		{
			name:    "nested block inside task",
			source:  "@dsl-version 1.2\n@task A\n@note Nested {\nText\n}\nText\n@endtask\nOutside",
			version: "1.2",
			parents: []int{0, 0, 2, 3, 3, 2, 0, 0},
			depths:  []int{0, 0, 1, 2, 2, 1, 0, 0},
		},
		{
			name:    "unknown version is a diagnostic",
			source:  "@dsl-version 2.0\nText",
			version: "2.0",
			code:    diagnostics.P014,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			grammars, assertions, err := prepareGrammar()
			if err != nil {
				t.Fatal(err)
			}
			result := model.Result{
				Format:      model.FormatVersion1,
				Document:    model.Document{Metadata: model.DocumentMetadata{ResourceDirs: []string{}}},
				Processing:  []model.Processing{{ID: "registry-test", Tool: "dsl-parser"}},
				Lines:       []model.Line{},
				Diagnostics: []model.Diagnostic{},
			}
			p := parser.New(strings.NewReader(test.source), &result, grammars, diagnostics.NewRegistry(), assertions)
			if err := p.Parse(); err != nil {
				t.Fatalf("Подготовленные зависимости привели к технической ошибке: %v", err)
			}
			if result.Document.DSLVersion == nil || *result.Document.DSLVersion != test.version {
				t.Fatalf("Версия входного документа не сохранена: %+v", result.Document)
			}
			if test.code != "" {
				if len(result.Diagnostics) != 1 {
					t.Fatalf("Ожидается одна диагностика %s: %+v", test.code, result.Diagnostics)
				}
				diagnostic := result.Diagnostics[0]
				if diagnostic.DiagnosticCode != test.code || !diagnostic.Fatal || diagnostic.SeverityLevel != diagnostics.SeverityError || diagnostic.Source != "registry-test" {
					t.Fatalf("Неверная диагностика неизвестной версии: %+v", diagnostic)
				}
				if !result.Document.HasErrors || result.Document.LineCount != nil || result.Lines == nil || len(result.Lines) != 0 {
					t.Fatal("Неизвестная версия не остановила разбор с ошибкой документа")
				}
				return
			}
			if result.Document.HasErrors || len(result.Diagnostics) != 0 {
				t.Fatalf("Корректный документ получил диагностики: %+v", result.Diagnostics)
			}
			if result.Document.LineCount == nil || *result.Document.LineCount != len(test.parents) || len(result.Lines) != len(test.parents) {
				t.Fatalf("Разобрано неверное число строк: %d", len(result.Lines))
			}
			for i, line := range result.Lines {
				parent := 0
				if line.ParentLine != nil {
					parent = *line.ParentLine
				}
				if parent != test.parents[i] || line.NestingLevel != test.depths[i] {
					t.Errorf("Строка %d: родитель %d, глубина %d; требуется %d, %d", line.Number, parent, line.NestingLevel, test.parents[i], test.depths[i])
				}
			}
		})
	}
}
