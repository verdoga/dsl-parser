package grammar

import (
	"slices"
	"testing"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
)

func TestContextPreservesSource(t *testing.T) {
	for _, source := range []string{"", " \t ", "\tПривет 🌍  "} {
		t.Run(source, func(t *testing.T) {
			c := NewContext(source)
			c.SetLine(model.Line{Raw: "другой результат"})
			c.AddElement(model.Element{Raw: "элемент"})
			if got := c.String(); got != source {
				t.Errorf("String() = %q, требуется %q", got, source)
			}
		})
	}
}

func TestContextReplacesLineAndAppendsElements(t *testing.T) {
	c := NewContext("исходник")
	assertContextLine(t, c.Line(), model.Line{})
	early := model.Element{ElementType: model.ElementTypeContent, Raw: "до", Start: 1, End: 3}
	c.AddElement(early)
	assertContextLine(t, c.Line(), model.Line{Elements: []model.Element{early}})
	parent, value := 1, "текст"
	element := model.Element{ElementType: model.ElementTypeContent, Raw: value, Value: &value,
		Start: 2, End: 7, ErrorIDs: []string{"d1", "d2"}}
	line := model.Line{Number: 2, LineType: model.LineTypeContent, NestingLevel: 1, ParentLine: &parent,
		Raw: " текст", LineEnding: model.LineEndingCRLF, HasErrors: true, Elements: []model.Element{element}}
	c.SetLine(line)
	assertContextLine(t, c.Line(), line)
	c.AddElement(early)
	line.Elements = []model.Element{element, early}
	assertContextLine(t, c.Line(), line)
	c.SetLine(model.Line{})
	assertContextLine(t, c.Line(), model.Line{})
}

func assertContextLine(t *testing.T, got, want model.Line) {
	t.Helper()
	if got.Number != want.Number || got.LineType != want.LineType || got.NestingLevel != want.NestingLevel ||
		got.ParentLine != want.ParentLine || got.Raw != want.Raw || got.LineEnding != want.LineEnding || got.HasErrors != want.HasErrors {
		t.Errorf("Line() = %+v, требуется %+v", got, want)
	}
	if !slices.EqualFunc(got.Elements, want.Elements, func(a, b model.Element) bool {
		return a.ElementType == b.ElementType && a.Raw == b.Raw && a.Value == b.Value &&
			a.Start == b.Start && a.End == b.End && slices.Equal(a.ErrorIDs, b.ErrorIDs)
	}) {
		t.Errorf("Элементы = %+v, требуется %+v", got.Elements, want.Elements)
	}
}

func TestContextDiagnosticsPreserveValuesOrderAndOwnSlice(t *testing.T) {
	c := NewContext("")
	if len(c.Diagnostics()) != 0 {
		t.Fatal("Новый контекст содержит диагностики")
	}
	location := model.Location{Start: model.Position{Line: 2, Column: 1}, End: model.Position{Line: 2, Column: 3}}
	first := model.Diagnostic{ID: "d2", Source: "parser", DiagnosticCode: diagnostics.P003,
		SeverityLevel: diagnostics.SeverityError, Message: "ошибка", DiagnosticScope: diagnostics.ScopeElement,
		Fatal: true, Location: &location, RelatedLocations: []model.Location{location}}
	second := model.Diagnostic{ID: "d1", Message: "другая диагностика"}
	c.AddDiagnostic(first)
	returned := c.Diagnostics()
	returned[0] = second
	c.AddDiagnostic(second)
	c.SetLine(model.Line{})
	for attempt := 0; attempt < 2; attempt++ {
		got := c.Diagnostics()
		if !slices.EqualFunc(got, []model.Diagnostic{first, second}, func(a, b model.Diagnostic) bool {
			return a.ID == b.ID && a.Source == b.Source && a.DiagnosticCode == b.DiagnosticCode &&
				a.SeverityLevel == b.SeverityLevel && a.Message == b.Message && a.DiagnosticScope == b.DiagnosticScope &&
				a.Fatal == b.Fatal && a.Location == b.Location && slices.Equal(a.RelatedLocations, b.RelatedLocations)
		}) {
			t.Fatalf("Diagnostics() = %+v, требуется [%+v %+v]", got, first, second)
		}
		got[0] = model.Diagnostic{}
	}
	if len(returned) != 1 || c.Line().HasErrors {
		t.Fatal("Накопление диагностик изменило прежний срез или признак ошибки строки")
	}
}

type contextTestRegistry func(diagnostics.Code) (diagnostics.Diagnostic, bool)

func (r contextTestRegistry) Lookup(code diagnostics.Code) (diagnostics.Diagnostic, bool) {
	return r(code)
}

func TestContextUsesInstalledDiagnosticRegistryAndReplacement(t *testing.T) {
	c := NewContext("")
	c.SetDiagnosticRegistry(diagnostics.NewRegistry())
	description, found := c.Lookup(diagnostics.P003)
	if !found || description == nil || description.Code() != diagnostics.P003 {
		t.Fatal("Не получено описание P003 из установленного реестра")
	}
	if got, found := c.Lookup("unknown"); found || got != nil {
		t.Fatal("Неизвестный код найден во встроенном реестре")
	}
	var calls []diagnostics.Code
	c.SetDiagnosticRegistry(contextTestRegistry(func(code diagnostics.Code) (diagnostics.Diagnostic, bool) {
		calls = append(calls, code)
		return nil, false
	}))
	if len(calls) != 0 {
		t.Fatal("Установка реестра вызвала поиск")
	}
	if got, found := c.Lookup(diagnostics.P003); found || got != nil {
		t.Fatal("После замены используется прежний реестр")
	}
	if !slices.Equal(calls, []diagnostics.Code{diagnostics.P003}) {
		t.Fatalf("Вызовы нового реестра = %v", calls)
	}
}

func TestContextAssertUsesOnlyFirstMatchingCheck(t *testing.T) {
	tests := []struct {
		name      string
		checks    []int
		id        AssertionID
		want      bool
		wantCalls int
	}{
		{"empty", nil, "target", false, 0},
		{"unknown", []int{1}, "unknown", false, 0},
		{"nil check", []int{-1}, "target", false, 0},
		{"true", []int{1}, "target", true, 1},
		{"false", []int{0}, "target", false, 1},
		{"first true", []int{1, 0}, "target", true, 1},
		{"first false", []int{0, 1}, "target", false, 1},
		{"first nil", []int{-1, 1}, "target", false, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, calls := NewContext(""), 0
			var assertions []Assertion
			if len(test.checks) > 0 {
				assertions = append(assertions, Assertion{ID: "unrelated", Check: func(AssertionInput) bool {
					t.Fatal("Вызвано постороннее утверждение")
					return false
				}})
			}
			for _, answer := range test.checks {
				assertion := Assertion{ID: "target"}
				if answer >= 0 {
					assertion.Check = func(AssertionInput) bool {
						calls++
						return answer == 1
					}
				}
				assertions = append(assertions, assertion)
			}
			c.SetAssertions(assertions)
			if calls != 0 {
				t.Fatal("SetAssertions вызвал проверку")
			}
			if got := c.Assert(test.id, AssertionInput{}); got != test.want || calls != test.wantCalls {
				t.Fatalf("Assert() = %t, вызовов %d; требуется %t, вызовов %d", got, calls, test.want, test.wantCalls)
			}
		})
	}
}

func TestContextAssertPassesInput(t *testing.T) {
	open, parent := model.Line{Number: 1}, model.Line{Number: 2}
	for _, test := range []struct {
		name         string
		open, parent *model.Line
	}{
		{"neither", nil, nil}, {"open only", &open, nil}, {"parent only", nil, &parent}, {"both", &open, &parent},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := NewContext("исходник")
			input := AssertionInput{Line: model.Line{Number: 3, Raw: "строка",
				Elements: []model.Element{{Raw: "строка", Start: 1, End: 7}}}, OpenBlock: test.open, CandidateParent: test.parent}
			calls := 0
			c.SetAssertions([]Assertion{{ID: "check", Check: func(got AssertionInput) bool {
				calls++
				assertContextLine(t, got.Line, input.Line)
				if got.OpenBlock != input.OpenBlock || got.CandidateParent != input.CandidateParent {
					t.Fatal("Изменены указатели входных данных")
				}
				return true
			}}})
			if !c.Assert("check", input) || calls != 1 {
				t.Fatal("Проверка не вызвана ровно один раз")
			}
			assertContextLine(t, c.Line(), model.Line{})
			if open.Number != 1 || parent.Number != 2 || input.Line.ParentLine != nil {
				t.Fatal("Assert изменил входные данные")
			}
		})
	}
}

func TestContextCopiesReplacesAndClearsAssertions(t *testing.T) {
	c := NewContext("")
	if c.Assert("old", AssertionInput{}) {
		t.Fatal("Новый контекст подтвердил неизвестное утверждение")
	}
	checks := []Assertion{{ID: "old", Check: func(AssertionInput) bool { return true }}}
	c.SetAssertions(checks)
	checks[0].ID, checks[0].Check = "changed", nil
	if !c.Assert("old", AssertionInput{}) || c.Assert("changed", AssertionInput{}) {
		t.Fatal("Контекст разделяет входной срез")
	}
	c.SetAssertions([]Assertion{{ID: "new", Check: func(AssertionInput) bool { return true }}})
	if c.Assert("old", AssertionInput{}) || !c.Assert("new", AssertionInput{}) {
		t.Fatal("Набор не заменён полностью")
	}
	for _, empty := range [][]Assertion{nil, {}} {
		c.SetAssertions([]Assertion{{ID: "new", Check: func(AssertionInput) bool { return true }}})
		c.SetAssertions(empty)
		if c.Assert("new", AssertionInput{}) {
			t.Fatal("Пустой набор не очистил утверждения")
		}
	}
}

func TestContextDetectionStoresLatestValuePerKind(t *testing.T) {
	c := NewContext("")
	if _, found := c.Detection("missing"); found {
		t.Fatal("Найден отсутствующий результат")
	}
	c.AddDetection("other", 9)
	for _, value := range []int{0, 4, -3, 0, 7} {
		c.AddDetection("target", value)
		if got, found := c.Detection("target"); !found || got != value {
			t.Fatalf("Detection() = %d, %t; требуется %d, true", got, found, value)
		}
		if got, found := c.Detection("other"); !found || got != 9 {
			t.Fatal("Изменён другой kind")
		}
	}
	if _, found := c.Detection("missing"); found {
		t.Fatal("После добавлений найден отсутствующий kind")
	}
}

func TestContextsKeepIndependentState(t *testing.T) {
	first, second := NewContext("первая"), NewContext("вторая")
	first.SetDiagnosticRegistry(diagnostics.NewRegistry())
	second.SetDiagnosticRegistry(contextTestRegistry(func(diagnostics.Code) (diagnostics.Diagnostic, bool) {
		return nil, false
	}))
	if description, found := first.Lookup(diagnostics.P003); !found || description == nil {
		t.Fatal("Установка реестра второго контекста изменила первый")
	}
	if description, found := second.Lookup(diagnostics.P003); found || description != nil {
		t.Fatal("Второй контекст использует реестр первого")
	}
	checks := []Assertion{{ID: "shared", Check: func(AssertionInput) bool { return true }}}
	first.SetAssertions(checks)
	second.SetAssertions(checks)
	first.SetLine(model.Line{Number: 1})
	first.AddElement(model.Element{Raw: "элемент"})
	first.AddDiagnostic(model.Diagnostic{ID: "d1"})
	first.AddDetection("kind", 5)
	first.SetAssertions(nil)
	assertContextLine(t, second.Line(), model.Line{})
	if len(second.Diagnostics()) != 0 {
		t.Fatal("Диагностики перешли в другой контекст")
	}
	if _, found := second.Detection("kind"); found {
		t.Fatal("Детектирование перешло в другой контекст")
	}
	if first.Assert("shared", AssertionInput{}) || !second.Assert("shared", AssertionInput{}) {
		t.Fatal("Наборы утверждений не независимы")
	}
	second.SetLine(model.Line{Number: 2})
	second.AddDetection("kind", 8)
	if first.String() != "первая" || second.String() != "вторая" || first.Line().Number != 1 {
		t.Fatal("Состояния строк смешались")
	}
	if got, found := first.Detection("kind"); !found || got != 5 {
		t.Fatal("Изменено детектирование первой строки")
	}
}
