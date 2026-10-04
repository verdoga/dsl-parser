package gr12

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/grammar"
	"github.com/verdoga/dsl-parser/model"
)

func TestOperandReadersPreserveFragmentsAndContinuation(t *testing.T) {
	identifier := func(c grammar.GrammarContext, column int) (model.Element, int, bool) {
		return readOperand(c, column, model.ElementTypeIdentifier)
	}
	for _, test := range []struct {
		name, source string
		read         func(grammar.GrammarContext, int) (model.Element, int, bool)
		from, next   int
		want         model.Element
	}{
		// DSL 2.3, 3.12, 3.13: ID выделяется отдельно от следующего операнда и скобки.
		{"task", "@task task-a", identifier, 6, 13, operandWant(model.ElementTypeIdentifier, "task-a", "task-a", 7)},
		{"fragment", "@fragment grammar-note {", identifier, 10, 23, operandWant(model.ElementTypeIdentifier, "grammar-note", "grammar-note", 11)},
		{"include extra", "@include grammar-note extra", identifier, 9, 22, operandWant(model.ElementTypeIdentifier, "grammar-note", "grammar-note", 10)},
		{"Unicode ID", "@task Ёж🌍\tother", identifier, 6, 10, operandWant(model.ElementTypeIdentifier, "Ёж🌍", "Ёж🌍", 7)},
		{"opening boundary", "x{", identifier, 1, 2, operandWant(model.ElementTypeIdentifier, "x", "x", 1)},
		{"closing boundary", "x}", identifier, 1, 2, operandWant(model.ElementTypeIdentifier, "x", "x", 1)},
		{"adjacent quoted operand", `audio"12"`, identifier, 1, 6, operandWant(model.ElementTypeIdentifier, "audio", "audio", 1)},
		{"escaped braces", `x\{y\}`, identifier, 1, 7, operandWant(model.ElementTypeIdentifier, `x\{y\}`, `x\{y\}`, 1)},
		{"even slashes", `x\\{`, identifier, 1, 4, operandWant(model.ElementTypeIdentifier, `x\\`, `x\\`, 1)},
		{"only boundary", " \t{", identifier, 1, 3, model.Element{}},
		{"no operand", "@task \t", identifier, 6, 8, model.Element{}},
		// DSL 3.1.5–3.1.9: SOURCE и синтетические варианты правил кавычек.
		{"bare SOURCE", "images/photo1.png extra", readMediaSource, 1, 18, operandWant(model.ElementTypeSource, "images/photo1.png", "images/photo1.png", 1)},
		{"quoted SOURCE", `"dialogue tracks/track 01.mp3" tail`, readMediaSource, 1, 31, operandWant(model.ElementTypeSource, `"dialogue tracks/track 01.mp3"`, "dialogue tracks/track 01.mp3", 1)},
		{"Unicode SOURCE", " \t\"Ёж🌍.mp3\" ", readMediaSource, 1, 12, operandWant(model.ElementTypeSource, `"Ёж🌍.mp3"`, "Ёж🌍.mp3", 3)},
		{"quoted braces", `"{x}"`, readMediaSource, 1, 6, operandWant(model.ElementTypeSource, `"{x}"`, "{x}", 1)},
		{"escaped quote", `"a\"b"`, readMediaSource, 1, 7, operandWant(model.ElementTypeSource, `"a\"b"`, `a\"b`, 1)},
		{"two slashes close", `"a\\" tail`, readMediaSource, 1, 6, operandWant(model.ElementTypeSource, `"a\\"`, `a\\`, 1)},
		{"three slashes escape", `"a\\\"b"`, readMediaSource, 1, 9, operandWant(model.ElementTypeSource, `"a\\\"b"`, `a\\\"b`, 1)},
		{"four slashes close", `"a\\\\"`, readMediaSource, 1, 8, operandWant(model.ElementTypeSource, `"a\\\\"`, `a\\\\`, 1)},
		{"slash then Unicode", `"\Ж"`, readMediaSource, 1, 5, operandWant(model.ElementTypeSource, `"\Ж"`, `\Ж`, 1)},
		{"unclosed SOURCE", `"a b`, readMediaSource, 1, 5, model.Element{}},
		{"escaped last quote", `"a\"`, readMediaSource, 1, 5, model.Element{}},
		{"media Windows slash escapes quote", `"C:\Audio\"`, readMediaSource, 1, 12, model.Element{}},
		{"SOURCE after Unicode prefix", `Ёж "a"`, readMediaSource, 3, 7, operandWant(model.ElementTypeSource, `"a"`, "a", 4)},
		{"absent SOURCE", " \t", readMediaSource, 1, 3, model.Element{}},
		// DSL 1.35: запятые и буквальные обратные косые черты в PATH.
		{"bare PATH", "Audio, next", readResourcePath, 1, 7, operandWant(model.ElementTypeResourcePath, "Audio", "Audio", 1)},
		{"spaces in PATH", " \tShared resources \t, next", readResourcePath, 1, 22, operandWant(model.ElementTypeResourcePath, "Shared resources", "Shared resources", 3)},
		{"comma in PATH", `"../Video, additional", next`, readResourcePath, 1, 24, operandWant(model.ElementTypeResourcePath, `"../Video, additional"`, "../Video, additional", 1)},
		{"Windows PATH", `"C:\Audio\", D:\Video`, readResourcePath, 1, 13, operandWant(model.ElementTypeResourcePath, `"C:\Audio\"`, `C:\Audio\`, 1)},
		{"Unicode PATH", " \"Ёж🌍, x\"\t, next", readResourcePath, 1, 12, operandWant(model.ElementTypeResourcePath, `"Ёж🌍, x"`, "Ёж🌍, x", 2)},
		{"PATH after Unicode prefix", "Ёж, Аудио", readResourcePath, 4, 10, operandWant(model.ElementTypeResourcePath, "Аудио", "Аудио", 5)},
		{"quoted inner spaces", `"  Audio  "`, readResourcePath, 1, 12, operandWant(model.ElementTypeResourcePath, `"  Audio  "`, "  Audio  ", 1)},
		{"literal braces", `C:\{Audio}\`, readResourcePath, 1, 12, operandWant(model.ElementTypeResourcePath, `C:\{Audio}\`, `C:\{Audio}\`, 1)},
		{"empty list item", ",Audio", readResourcePath, 1, 2, model.Element{}},
		{"unclosed PATH", `"Audio, Video`, readResourcePath, 1, 14, model.Element{}},
		{"text after quote", `"Audio"x, Video`, readResourcePath, 1, 10, model.Element{}},
		{"quotes in bare path", `A"u"dio, Video`, readResourcePath, 1, 9, model.Element{}},
		{"two quoted values", `"A""B", Video`, readResourcePath, 1, 8, model.Element{}},
		{"no PATH", " \t", readResourcePath, 1, 3, model.Element{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			context := newOperandContext(t, test.source, "")
			got, next, ok := test.read(context, test.from)
			if ok != (test.want.ElementType != "") || next != test.next {
				t.Errorf("ok/next = %t/%d, требуется %t/%d", ok, next, test.want.ElementType != "", test.next)
			}
			assertOperandElement(t, got, test.want)
			if len(context.Line().Elements) != 0 || len(context.Diagnostics()) != 0 || context.String() != test.source || context.Line().Raw != test.source {
				t.Fatal("Чтение операнда изменило контекст")
			}
		})
	}
}

func TestReadOperandKeepsRequestedKindAndOriginalValue(t *testing.T) {
	for _, test := range []struct {
		source, raw string
		kind        model.ElementType
		column      int
	}{
		{"@dsl-version 1.2", "1.2", model.ElementTypeVersion, 14},
		{"@order 0020", "0020", model.ElementTypeNumber, 8},
		{"@document-id Scenario-7F3A91C2", "Scenario-7F3A91C2", model.ElementTypeIdentifier, 14},
	} {
		t.Run(test.source, func(t *testing.T) {
			got, next, ok := readOperand(newOperandContext(t, test.source, ""), test.column, test.kind)
			want := operandWant(test.kind, test.raw, test.raw, test.column)
			assertOperandElement(t, got, want)
			if !ok || next != want.End {
				t.Fatalf("Операнд не завершён: ok=%t, next=%d", ok, next)
			}
		})
	}
}

func TestParseMediaFromDSLAndGrammarMutations(t *testing.T) {
	for _, test := range []struct {
		source, typeRaw, typeValue, sourceRaw, sourceValue string
		typeStart, sourceStart                             int
		codes                                              []diagnostics.Code
		locations                                          [][2]int
	}{
		// Все четыре примера DSL 3.1 и регистронезависимость по 1.11.
		{`@media audio "dialogue tracks/track 01.mp3"`, "audio", "audio", `"dialogue tracks/track 01.mp3"`, "dialogue tracks/track 01.mp3", 8, 14, nil, nil},
		{"@media image images/photo1.png", "image", "image", "images/photo1.png", "images/photo1.png", 8, 14, nil, nil},
		{`@media video "unit 2/videos/promo.mp4"`, "video", "video", `"unit 2/videos/promo.mp4"`, "unit 2/videos/promo.mp4", 8, 14, nil, nil},
		{"@media audio 12", "audio", "audio", "12", "12", 8, 14, nil, nil},
		{"@MEDIA AuDiO 0012", "AuDiO", "audio", "0012", "0012", 8, 14, nil, nil},
		{"@media VIDEO 12", "VIDEO", "video", "12", "12", 8, 14, nil, nil},
		{"@media Image 12", "Image", "image", "12", "12", 8, 14, nil, nil},
		{"@media other 12", "other", "", "12", "12", 8, 14, nil, nil},
		{"@media Медиа 12", "Медиа", "", "12", "12", 8, 14, nil, nil},
		{`@media audio "12"`, "audio", "audio", `"12"`, "12", 8, 14, nil, nil},
		{`@media audio "a\"b\\.mp3"`, "audio", "audio", `"a\"b\\.mp3"`, `a\"b\\.mp3`, 8, 14, nil, nil},
		{" \t@media image \"Ёж🌍.png\" \t", "image", "image", `"Ёж🌍.png"`, "Ёж🌍.png", 10, 16, nil, nil},
		{"@media", "", "", "", "", 0, 0, []diagnostics.Code{diagnostics.P006}, [][2]int{{7, 7}}},
		{"@media \t", "", "", "", "", 0, 0, []diagnostics.Code{diagnostics.P006}, [][2]int{{9, 9}}},
		{"@media audio", "audio", "audio", "", "", 8, 0, []diagnostics.Code{diagnostics.P006}, [][2]int{{13, 13}}},
		{"@media audio  ", "audio", "audio", "", "", 8, 0, []diagnostics.Code{diagnostics.P006}, [][2]int{{15, 15}}},
		{"@media audio {", "audio", "audio", "", "", 8, 0, []diagnostics.Code{diagnostics.P006}, [][2]int{{14, 14}}},
		{"@media\taudio\t12", "audio", "audio", "12", "12", 8, 14, []diagnostics.Code{diagnostics.P004, diagnostics.P004}, [][2]int{{7, 8}, {13, 14}}},
		{`@media audio"12"`, "audio", "audio", `"12"`, "12", 8, 13, []diagnostics.Code{diagnostics.P004}, [][2]int{{13, 14}}},
		{"@media audio 12 extra", "audio", "audio", "12", "12", 8, 14, []diagnostics.Code{diagnostics.P007}, [][2]int{{17, 22}}},
		{"@media audio 12 {", "audio", "audio", "12", "12", 8, 14, []diagnostics.Code{diagnostics.P005}, [][2]int{{1, 18}}},
		{`@media audio "12" {`, "audio", "audio", `"12"`, "12", 8, 14, []diagnostics.Code{diagnostics.P005}, [][2]int{{1, 20}}},
		{"@media audio 12{", "audio", "audio", "12", "12", 8, 14, []diagnostics.Code{diagnostics.P004, diagnostics.P005}, [][2]int{{16, 17}, {1, 17}}},
		{"@media audio 12\t{", "audio", "audio", "12", "12", 8, 14, []diagnostics.Code{diagnostics.P004, diagnostics.P005}, [][2]int{{16, 17}, {1, 18}}},
		{`@media audio "12"{`, "audio", "audio", `"12"`, "12", 8, 14, []diagnostics.Code{diagnostics.P004, diagnostics.P005}, [][2]int{{18, 19}, {1, 19}}},
		{`@media image "Ёж🌍" {tail`, "image", "image", `"Ёж🌍"`, "Ёж🌍", 8, 14, []diagnostics.Code{diagnostics.P005}, [][2]int{{1, 25}}},
		{`@media audio "{12}"`, "audio", "audio", `"{12}"`, "{12}", 8, 14, nil, nil},
		{`@media audio 12\{`, "audio", "audio", `12\{`, `12\{`, 8, 14, nil, nil},
		{`@media audio "12"x`, "audio", "audio", `"12"`, "12", 8, 14, []diagnostics.Code{diagnostics.P007}, [][2]int{{18, 19}}},
		{`@media audio "unclosed`, "audio", "audio", "", "", 8, 0, nil, nil},
		{`@media audio "a\"`, "audio", "audio", "", "", 8, 0, nil, nil},
		// Пустое выделенное значение не подменяется отсутствующим операндом.
		{`@media audio ""`, "audio", "audio", `""`, "", 8, 14, nil, nil},
		{`@media audio "  "`, "audio", "audio", `"  "`, "  ", 8, 14, nil, nil},
	} {
		t.Run(test.source, func(t *testing.T) {
			context := newOperandContext(t, test.source, "media")
			want := append([]model.Element{}, context.Line().Elements...)
			if test.typeRaw != "" {
				e := operandWant(model.ElementTypeMediaType, test.typeRaw, test.typeValue, test.typeStart)
				if test.typeValue == "" {
					e.Value = nil
				}
				want = append(want, e)
			}
			if test.sourceRaw != "" {
				want = append(want, operandWant(model.ElementTypeSource, test.sourceRaw, test.sourceValue, test.sourceStart))
			}
			parseMedia(context)
			assertOperandResult(t, context, test.source, want, test.codes, test.locations)
		})
	}
}

func TestParseResourceDirsFromDSLAndGrammarMutations(t *testing.T) {
	for _, test := range []struct {
		source string
		paths  [][2]string
		codes  []diagnostics.Code
	}{
		// Полный список из DSL 1.35 и производные случаи по 1.35.4–1.35.8.
		{`@resource-dir Audio, "../Shared resources", "../Video, additional"`, [][2]string{{"Audio", "Audio"}, {`"../Shared resources"`, "../Shared resources"}, {`"../Video, additional"`, "../Video, additional"}}, nil},
		{" \t@resource-dir  Shared resources,\t\"Ёж🌍, x\" \t", [][2]string{{"Shared resources", "Shared resources"}, {`"Ёж🌍, x"`, "Ёж🌍, x"}}, nil},
		{`@resource-dir "C:\Audio\", D:\Video, C:\{Text}\`, [][2]string{{`"C:\Audio\"`, `C:\Audio\`}, {`D:\Video`, `D:\Video`}, {`C:\{Text}\`, `C:\{Text}\`}}, nil},
		{`@resource-dir "  Audio  ", A\,B`, [][2]string{{`"  Audio  "`, "  Audio  "}, {`A\`, `A\`}, {"B", "B"}}, nil},
		{"@resource-dir A\tB, Audio, Audio", [][2]string{{"A\tB", "A\tB"}, {"Audio", "Audio"}, {"Audio", "Audio"}}, nil},
		{"@resource-dir\tAudio", [][2]string{{"Audio", "Audio"}}, []diagnostics.Code{diagnostics.P004}},
		{`@resource-dir"Audio"`, [][2]string{{`"Audio"`, "Audio"}}, []diagnostics.Code{diagnostics.P004}},
		{"@resource-dir", nil, nil}, {"@resource-dir \t", nil, nil},
		{"@resource-dir ,Audio,,Video,", [][2]string{{"Audio", "Audio"}, {"Video", "Video"}}, nil},
		{`@resource-dir Audio, "unfinished, Video`, [][2]string{{"Audio", "Audio"}}, nil},
		{`@resource-dir "Audio"x, Video`, [][2]string{{"Video", "Video"}}, nil},
		{`@resource-dir ""`, [][2]string{{`""`, ""}}, nil},
	} {
		t.Run(test.source, func(t *testing.T) {
			context := newOperandContext(t, test.source, "resource-dir")
			want := append([]model.Element{}, context.Line().Elements...)
			remaining, column := test.source[want[0].End-1:], want[0].End
			for _, path := range test.paths {
				index := strings.Index(remaining, path[0])
				if index < 0 {
					t.Fatalf("Фрагмент ожидания %q отсутствует в примере", path[0])
				}
				start := column + utf8.RuneCountInString(remaining[:index])
				e := operandWant(model.ElementTypeResourcePath, path[0], path[1], start)
				want = append(want, e)
				remaining, column = remaining[index+len(path[0]):], e.End
			}
			parseResourceDirs(context)
			assertOperandResult(t, context, test.source, want, test.codes, [][2]int{{14, 15}})
		})
	}
}

func TestOperandReadersAndParsersWithoutInput(t *testing.T) {
	for _, context := range []grammar.GrammarContext{nil, newOperandContext(t, "", "")} {
		parseMedia(context)
		parseResourceDirs(context)
		for _, column := range []int{-1, 0, 1, 2} {
			for _, read := range []func(grammar.GrammarContext, int) (model.Element, int, bool){
				readMediaSource, readResourcePath,
				func(c grammar.GrammarContext, column int) (model.Element, int, bool) {
					return readOperand(c, column, model.ElementTypeIdentifier)
				},
			} {
				element, next, ok := read(context, column)
				assertOperandElement(t, element, model.Element{})
				if ok || next != column {
					t.Fatalf("Выдуманный операнд: ok=%t, next=%d, column=%d", ok, next, column)
				}
			}
		}
	}
}

func newOperandContext(t *testing.T, source, tag string) grammar.GrammarContext {
	t.Helper()
	context := grammar.NewContext(source)
	context.SetDiagnosticRegistry(diagnostics.NewRegistry())
	context.SetLine(model.Line{Number: 19, Raw: source, LineType: model.LineTypeTag})
	if tag != "" {
		start := utf8.RuneCountInString(source) - utf8.RuneCountInString(strings.TrimLeft(source, " \t")) + 1
		raw := string([]rune(source)[start-1 : start+len(tag)])
		context.AddElement(operandWant(model.ElementTypeTag, raw, tag, start))
	}
	return context
}

func operandWant(kind model.ElementType, raw, value string, start int) model.Element {
	return model.Element{ElementType: kind, Raw: raw, Value: &value, Start: start, End: start + utf8.RuneCountInString(raw), ErrorIDs: []string{}}
}

func assertOperandElement(t *testing.T, got, want model.Element) {
	t.Helper()
	if got.ElementType != want.ElementType || got.Raw != want.Raw || got.Start != want.Start || got.End != want.End || len(got.ErrorIDs) != 0 {
		t.Errorf("Элемент %+v, требуется %+v", got, want)
	}
	if (got.Value == nil) != (want.Value == nil) || (got.Value != nil && want.Value != nil && *got.Value != *want.Value) {
		t.Errorf("Неверное значение элемента %q: получено %v, требуется %v", got.Raw, got.Value, want.Value)
	}
}

func assertOperandResult(t *testing.T, context grammar.GrammarContext, source string, want []model.Element, codes []diagnostics.Code, locations [][2]int) {
	t.Helper()
	line := context.Line()
	if len(line.Elements) != len(want) {
		t.Fatalf("Элементов %d, требуется %d: %+v", len(line.Elements), len(want), line.Elements)
	}
	for i, element := range line.Elements {
		assertOperandElement(t, element, want[i])
	}
	if context.String() != source || line.Raw != source || line.Number != 19 || line.LineType != model.LineTypeTag || line.HasErrors || line.ParentLine != nil || line.NestingLevel != 0 {
		t.Fatal("Разбор операндов изменил исходник или свойства физической строки")
	}
	got := context.Diagnostics()
	if len(got) != len(codes) {
		t.Fatalf("Диагностики %+v, требуются %v", got, codes)
	}
	for i, d := range got {
		location := model.Location{Start: model.Position{Line: 19, Column: locations[i][0]}, End: model.Position{Line: 19, Column: locations[i][1]}}
		if d.DiagnosticCode != codes[i] || d.DiagnosticScope != diagnostics.ScopeLine || d.Location == nil || *d.Location != location || d.ID != "" || d.Source != "" {
			t.Errorf("Диагностика %+v, требуется %s в %+v", d, codes[i], location)
		}
	}
}
