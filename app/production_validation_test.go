package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
)

const productionFixtureSHA256 = "6f3a5d335c64f38a4b7a50745183789035e0daa1301118cce1d95cbbb21f1329"

type productionDiagnostic struct {
	code             diagnostics.Code
	line, start, end int
	raw              string
}

type productionElement struct {
	kind       model.ElementType
	raw, value string
	start      int
}

func TestProductionOriginalAndCorrectedMedia(t *testing.T) {
	source := productionFixture(t)
	original := runProductionJSON(t, source, productionOriginalErrors())
	for _, number := range []int{69, 116} {
		assertProductionElements(t, original.Lines[number-1], model.LineTypeTag,
			productionElement{model.ElementTypeTag, "@media", "media", 1},
			productionElement{model.ElementTypeUnparsed, "vidio", "", 8},
			productionElement{model.ElementTypeSource, `"Unit 07"`, "Unit 07", 14})
	}
	if original.Document.Metadata.DocumentID == nil || *original.Document.Metadata.DocumentID != "NKRY" || original.Document.Metadata.Order == nil || *original.Document.Metadata.Order != "10" {
		t.Fatal("Исходные метаданные производственного файла потеряны")
	}
	corrected := bytes.ReplaceAll(source, []byte(`@media vidio "Unit 07"`), []byte(`@media video "Unit 07"`))
	clean := runProductionJSON(t, corrected, nil)
	for _, number := range []int{62, 69, 116} {
		assertProductionElements(t, clean.Lines[number-1], model.LineTypeTag,
			productionElement{model.ElementTypeTag, "@media", "media", 1},
			productionElement{model.ElementTypeMediaType, "video", "video", 8},
			productionElement{model.ElementTypeSource, `"Unit 07"`, "Unit 07", 14})
	}
	assertProductionUnchanged(t, original, clean, 0, 0, 69, 116)
	assertProductionMetadata(t, clean, original.Document.Metadata)
}

func TestProductionMediaTypesAndMissingOperands(t *testing.T) {
	source := productionFixture(t)
	base := runProductionJSON(t, source, productionOriginalErrors())
	for _, tc := range []struct{ raw, value string }{
		{"audio", "audio"}, {"video", "video"}, {"image", "image"},
		{"AUDIO", "audio"}, {"VIDEO", "video"}, {"IMAGE", "image"},
		{"AuDiO", "audio"}, {"ViDeO", "video"}, {"Image", "image"},
		{"vidio", ""}, {"other", ""}, {"VIDEOx", ""}, {"123", ""}, {"Медиа", ""},
		{"vіdeo", ""}, {"audio,video", ""}, {"-", ""}, {"NULL", ""}, {`video\x`, ""},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			want := productionOriginalErrors()
			kind := model.ElementTypeMediaType
			end := 8 + utf8.RuneCountInString(tc.raw)
			if tc.value == "" {
				kind = model.ElementTypeUnparsed
				want = append(want, productionDiagnostic{diagnostics.P006, 62, 8, end, tc.raw})
			}
			result := runProductionJSON(t, productionReplace(source, 62, "@media "+tc.raw+` "Unit 07"`), want)
			assertProductionElements(t, result.Lines[61], model.LineTypeTag,
				productionElement{model.ElementTypeTag, "@media", "media", 1},
				productionElement{kind, tc.raw, tc.value, 8},
				productionElement{model.ElementTypeSource, `"Unit 07"`, "Unit 07", end + 1})
			assertProductionUnchanged(t, base, result, 0, 0, 62)
			assertProductionMetadata(t, result, base.Document.Metadata)
		})
	}
	for _, tc := range []struct {
		raw      string
		parts    []productionElement
		problems []productionDiagnostic
	}{
		{"@media", []productionElement{{model.ElementTypeUnparsed, "", "", 7}}, []productionDiagnostic{{diagnostics.P006, 62, 7, 7, ""}}},
		{"@media \t", []productionElement{{model.ElementTypeUnparsed, "", "", 9}}, []productionDiagnostic{{diagnostics.P006, 62, 9, 9, ""}}},
		{"@media video", []productionElement{{model.ElementTypeMediaType, "video", "video", 8}, {model.ElementTypeUnparsed, "", "", 13}}, []productionDiagnostic{{diagnostics.P006, 62, 13, 13, ""}}},
		{"@media vidio", []productionElement{{model.ElementTypeUnparsed, "vidio", "", 8}, {model.ElementTypeUnparsed, "", "", 13}}, []productionDiagnostic{{diagnostics.P006, 62, 8, 13, "vidio"}, {diagnostics.P006, 62, 13, 13, ""}}},
		{`@media video "Ёж🌍`, []productionElement{{model.ElementTypeMediaType, "video", "video", 8}, {model.ElementTypeUnparsed, `"Ёж🌍`, "", 14}}, []productionDiagnostic{{diagnostics.P015, 62, 14, 18, `"Ёж🌍`}}},
		{`@media vidio "Ёж🌍`, []productionElement{{model.ElementTypeUnparsed, "vidio", "", 8}, {model.ElementTypeUnparsed, `"Ёж🌍`, "", 14}}, []productionDiagnostic{{diagnostics.P006, 62, 8, 13, "vidio"}, {diagnostics.P015, 62, 14, 18, `"Ёж🌍`}}},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			result := runProductionJSON(t, productionReplace(source, 62, tc.raw), append(productionOriginalErrors(), tc.problems...))
			parts := append([]productionElement{{model.ElementTypeTag, "@media", "media", 1}}, tc.parts...)
			assertProductionElements(t, result.Lines[61], model.LineTypeTag, parts...)
			assertProductionUnchanged(t, base, result, 0, 0, 62)
			assertProductionMetadata(t, result, base.Document.Metadata)
		})
	}
	t.Run("Unicode columns with indentation", func(t *testing.T) {
		result := runProductionJSON(t, productionReplace(source, 62, " \t@media Медиа \"Ёж🌍\" \t"),
			append(productionOriginalErrors(), productionDiagnostic{diagnostics.P006, 62, 10, 15, "Медиа"}))
		assertProductionElements(t, result.Lines[61], model.LineTypeTag,
			productionElement{model.ElementTypeTag, "@media", "media", 3},
			productionElement{model.ElementTypeUnparsed, "Медиа", "", 10},
			productionElement{model.ElementTypeSource, `"Ёж🌍"`, "Ёж🌍", 16})
		assertProductionUnchanged(t, base, result, 0, 0, 62)
	})
}

func TestProductionOrderValuesAndMetadata(t *testing.T) {
	source := productionFixture(t)
	base := runProductionJSON(t, source, productionOriginalErrors())
	type orderCase struct {
		name, raw string
		valid     bool
	}
	cases := []orderCase{{"one", "1", true}, {"leading zeros", "0010", true},
		{"large production mutation", "999999999999999999999999999999999999999999", true},
		{"long number", strings.Repeat("1234567890", 410), true},
		{"long zero prefix", strings.Repeat("0", 4096) + "1", true}}
	for _, raw := range []string{"", "garbage", "vidio", "0", "000", "-1", "-10", "1.5", "NaN", "10abc", "+1", "+", "-", "1e3", "0x10", "0b10", "0o10", "1_000", "１２", "١٠", `10\{x\}`} {
		cases = append(cases, orderCase{"invalid " + raw, raw, false})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, start := "@order "+tc.raw, 8
			if tc.raw == "" {
				raw, start = "@order", 7
			}
			want, metadata := productionOriginalErrors(), base.Document.Metadata
			kind := model.ElementTypeNumber
			if tc.valid {
				metadata.Order = &tc.raw
			} else {
				kind, metadata.Order = model.ElementTypeUnparsed, nil
				want = append(want, productionDiagnostic{diagnostics.P006, 4, start, start + utf8.RuneCountInString(tc.raw), tc.raw})
			}
			result := runProductionJSON(t, productionReplace(source, 4, raw), want)
			assertProductionElements(t, result.Lines[3], model.LineTypeTag,
				productionElement{model.ElementTypeTag, "@order", "order", 1}, productionElement{kind, tc.raw, tc.raw, start})
			assertProductionMetadata(t, result, metadata)
			assertProductionUnchanged(t, base, result, 0, 0, 4)
		})
	}
	t.Run("missing after whitespace", func(t *testing.T) {
		result := runProductionJSON(t, productionReplace(source, 4, "@order \t"), append(productionOriginalErrors(), productionDiagnostic{diagnostics.P006, 4, 9, 9, ""}))
		assertProductionElements(t, result.Lines[3], model.LineTypeTag,
			productionElement{model.ElementTypeTag, "@order", "order", 1}, productionElement{model.ElementTypeUnparsed, "", "", 9})
		metadata := base.Document.Metadata
		metadata.Order = nil
		assertProductionMetadata(t, result, metadata)
		assertProductionUnchanged(t, base, result, 0, 0, 4)
	})
}

func TestProductionRepeatedVersionsAndTextControls(t *testing.T) {
	source := productionFixture(t)
	base := runProductionJSON(t, source, productionOriginalErrors())
	repeats := []string{
		"@dsl-version 1.2", "@dsl-version 2.0", "@dsl-version banana", "@dsl-version 1.x", "@dsl-version 1.2junk",
		"@dsl-version 1.1", "@dsl-version 01.2", "@dsl-version 1.20", "@dsl-version 1.2.0", "@dsl-version -1", "@dsl-version ١.٢",
		"@dsl-version", "@dsl-version \t", `@dsl-version "unclosed`, "@dsl-version 1.2 {", "@dsl-version 1.2 }",
		"@dsl-version 1.2 extra", "@dsl-version 1.2 @task B", "@dsl-version\t2.0", "@dsl-version{", "@dsl-version}",
		`@dsl-version"unclosed`, " \t@DsL-VeRsIoN banana🌍 \t",
	}
	for _, raw := range repeats {
		t.Run(raw, func(t *testing.T) {
			problem := productionWholeError(diagnostics.P003, 7, raw)
			result := runProductionJSON(t, productionReplace(source, 7, raw), append(productionOriginalErrors(), problem))
			assertProductionElements(t, result.Lines[6], model.LineTypeInvalid, productionElement{model.ElementTypeUnparsed, problem.raw, "", problem.start})
			assertProductionMetadata(t, result, base.Document.Metadata)
			assertProductionUnchanged(t, base, result, 0, 0, 7)
		})
	}
	// Последовательные повторы на корневом уровне и внутри существующего @choice.
	// Сравнение всех исходных строк учитывает сдвиг номеров и родительских ссылок.
	for _, at := range []int{7, 13} {
		t.Run(fmt.Sprintf("consecutive at line %d", at), func(t *testing.T) {
			want := productionOriginalErrors()
			for i := range want {
				want[i].line += len(repeats)
			}
			for i, raw := range repeats {
				want = append(want, productionWholeError(diagnostics.P003, at+i, raw))
			}
			result := runProductionJSON(t, productionInsert(source, at, repeats), want)
			for i, raw := range repeats {
				problem := productionWholeError(diagnostics.P003, at+i, raw)
				line := result.Lines[at+i-1]
				assertProductionElements(t, line, model.LineTypeInvalid, productionElement{model.ElementTypeUnparsed, problem.raw, "", problem.start})
				if !reflect.DeepEqual(line.ParentLine, base.Lines[at-1].ParentLine) || line.NestingLevel != base.Lines[at-1].NestingLevel {
					t.Fatal("Повтор версии изменил текущий блок или логического родителя")
				}
			}
			assertProductionMetadata(t, result, base.Document.Metadata)
			assertProductionUnchanged(t, base, result, at, len(repeats))
		})
	}
	for _, tc := range []struct {
		line int
		raw  string
	}{
		{7, `\@dsl-version 2.0`}, {7, `\@DSL-VERSION "unclosed`},
		{22, "@dsl-version banana"}, {22, `@dsl-version "unclosed`}, {22, "@dsl-version"},
	} {
		t.Run(fmt.Sprintf("content %d %s", tc.line, tc.raw), func(t *testing.T) {
			result := runProductionJSON(t, productionReplace(source, tc.line, tc.raw), productionOriginalErrors())
			assertProductionElements(t, result.Lines[tc.line-1], model.LineTypeContent, productionElement{model.ElementTypeContent, tc.raw, tc.raw, 1})
			if !reflect.DeepEqual(result.Lines[tc.line-1].ParentLine, base.Lines[tc.line-1].ParentLine) {
				t.Fatal("Изменён родитель текстового содержимого")
			}
			assertProductionMetadata(t, result, base.Document.Metadata)
			assertProductionUnchanged(t, base, result, 0, 0, tc.line)
		})
	}
}

func TestProductionFirstVersionRulesCannotBeBypassed(t *testing.T) {
	source := productionFixture(t)
	base := runProductionJSON(t, source, productionOriginalErrors())
	for _, tc := range []struct {
		raw, tag            string
		start, versionStart int
	}{
		{"@DSL-VERSION 1.2", "@DSL-VERSION", 1, 14},
		{" \t@DsL-VeRsIoN   1.2 \t", "@DsL-VeRsIoN", 3, 18},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			result := runProductionJSON(t, productionReplace(source, 1, tc.raw), productionOriginalErrors())
			assertProductionElements(t, result.Lines[0], model.LineTypeTag,
				productionElement{model.ElementTypeTag, tc.tag, "dsl-version", tc.start}, productionElement{model.ElementTypeVersion, "1.2", "1.2", tc.versionStart})
			assertProductionMetadata(t, result, base.Document.Metadata)
			assertProductionUnchanged(t, base, result, 0, 0, 1)
		})
	}
	for _, tc := range []struct {
		raw  string
		code diagnostics.Code
	}{
		{"@dsl-version 2.0", diagnostics.P014}, {"@dsl-version 1.1", diagnostics.P014},
		{"@dsl-version banana", diagnostics.P014}, {"@dsl-version 1.x", diagnostics.P014}, {"@dsl-version 1.2junk", diagnostics.P014},
		{"", diagnostics.P013}, {"@dsl-version", diagnostics.P013}, {"@dsl-version\t1.2", diagnostics.P013},
		{"@dsl-version 1.2 extra", diagnostics.P013}, {"@dsl-versionx 1.2", diagnostics.P013}, {`\@dsl-version 1.2`, diagnostics.P013},
	} {
		t.Run("fatal "+tc.raw, func(t *testing.T) {
			copy := productionReplace(source, 1, tc.raw)
			copy = productionReplace(copy, 7, "@dsl-version 1.2")
			copy = productionReplace(copy, 9, "@dsl-version 2.0")
			path := writeRunSource(t, t.TempDir(), "Work.txt", string(copy))
			code, stdout, stderr := captureRunOutput(t, func() int { return Run([]string{path}, "production-test") })
			// storage не сохраняет JSON при fatal: проверяем явный код ошибки
			// в отчёте, единственный отказ и отсутствие выходного файла.
			if code != 1 || stderr != "" || strings.Count(stdout, "фатальная ошибка:") != 1 || !strings.Contains(stdout, "фатальная ошибка: "+string(tc.code)) || !strings.Contains(stdout, "ошибок: 1") || !strings.Contains(stdout, "ИТОГО | всего: 1 | успех: 0 | ошибка: 1") {
				t.Fatalf("Неверный ранний отказ: code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
			}
			if _, err := os.Stat(strings.TrimSuffix(path, ".txt") + ".json"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("Поздний повтор обошёл ранний отказ: JSON существует, err=%v", err)
			}
			unchanged, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(unchanged, copy) {
				t.Fatal("Ранний отказ изменил входную копию")
			}
		})
	}
}

func TestProductionHeadingErrorsAndValidLevels(t *testing.T) {
	source := productionFixture(t)
	base := runProductionJSON(t, source, productionOriginalErrors())
	// Девять отрицательных вариантов из первоначального аудита.
	for _, raw := range []string{
		"#### 2 WHILE YOU WATCH", "##### 2 WHILE YOU WATCH", "########## 2 WHILE YOU WATCH",
		"#x 2 WHILE YOU WATCH", "##x 2 WHILE YOU WATCH", "###x 2 WHILE YOU WATCH",
		"###2 WHILE YOU WATCH", "###\t2 WHILE YOU WATCH", "###",
		" \t###Ёж🌍 \t", // Дополнительно: точные Unicode-колонки и краевые пробелы.
	} {
		t.Run(raw, func(t *testing.T) {
			problem := productionWholeError(diagnostics.P003, 59, raw)
			result := runProductionJSON(t, productionReplace(source, 59, raw), append(productionOriginalErrors(), problem))
			assertProductionElements(t, result.Lines[58], model.LineTypeInvalid, productionElement{model.ElementTypeUnparsed, problem.raw, "", problem.start})
			// Invalid не завершает task c; следующий @task по-прежнему открывается корректно.
			if result.Lines[58].ParentLine == nil || *result.Lines[58].ParentLine != 30 || result.Lines[59].ParentLine == nil || *result.Lines[59].ParentLine != 30 || result.Lines[59].LineType != model.LineTypeBlank {
				t.Fatal("Ошибочный заголовок нарушил состояние текущего задания")
			}
			assertProductionMetadata(t, result, base.Document.Metadata)
			assertProductionUnchanged(t, base, result, 0, 0, 59, 60)
		})
	}
	for level := 1; level <= 3; level++ {
		t.Run(fmt.Sprintf("valid level %d", level), func(t *testing.T) {
			marker, title := strings.Repeat("#", level), "2 WHILE YOU WATCH"
			result := runProductionJSON(t, productionReplace(source, 59, marker+" "+title), productionOriginalErrors())
			assertProductionElements(t, result.Lines[58], model.LineTypeHeading,
				productionElement{model.ElementTypeHeadingLevel, marker, fmt.Sprint(level), 1}, productionElement{model.ElementTypeTitle, title, title, level + 2})
			metadata := base.Document.Metadata
			if level == 1 {
				metadata.Title = nil // Второе объявление одиночного заголовка первого уровня.
			} else if level == 2 {
				metadata.Subtitle = &title
			}
			assertProductionMetadata(t, result, metadata)
			assertProductionUnchanged(t, base, result, 0, 0, 59, 60)
		})
	}
}

func productionWholeError(code diagnostics.Code, number int, raw string) productionDiagnostic {
	start := utf8.RuneCountInString(raw) - utf8.RuneCountInString(strings.TrimLeft(raw, " \t")) + 1
	trimmed := strings.Trim(raw, " \t")
	return productionDiagnostic{code, number, start, start + utf8.RuneCountInString(trimmed), trimmed}
}

func productionFixture(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("testdata", "Work.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 4706 || bytes.Count(data, []byte{'\n'}) != 170 || fmt.Sprintf("%x", sha256.Sum256(data)) != productionFixtureSHA256 {
		t.Fatal("Производственный fixture отличается от приложенного файла")
	}
	t.Cleanup(func() {
		current, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(current, data) {
			t.Errorf("Производственный fixture изменён: %v", err)
		}
	})
	return data
}

func productionOriginalErrors() []productionDiagnostic {
	return []productionDiagnostic{{diagnostics.P006, 69, 8, 13, "vidio"}, {diagnostics.P006, 116, 8, 13, "vidio"}}
}

func productionReplace(source []byte, number int, raw string) []byte {
	lines := strings.Split(string(source), "\n")
	lines[number-1] = raw
	return []byte(strings.Join(lines, "\n"))
}

func productionInsert(source []byte, number int, added []string) []byte {
	lines := strings.Split(string(source), "\n")
	return []byte(strings.Join(append(append(slices.Clone(lines[:number-1]), added...), lines[number-1:]...), "\n"))
}

// Все входные копии и JSON находятся в t.TempDir; fixture только читается.
func runProductionJSON(t *testing.T, source []byte, want []productionDiagnostic) model.Result {
	t.Helper()
	path := writeRunSource(t, t.TempDir(), "Work.txt", string(source))
	code, stdout, stderr := captureRunOutput(t, func() int { return Run([]string{path}, "production-test") })
	if code != 0 || stderr != "" {
		t.Fatalf("app.Run: code=%d, stdout=%q, stderr=%q", code, stdout, stderr)
	}
	payload, err := os.ReadFile(strings.TrimSuffix(path, ".txt") + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var result model.Result
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	if result.Format != model.FormatVersion1 || result.Document.FilePath == nil || *result.Document.FilePath != path || len(result.Processing) != 1 || result.Processing[0].ID == "" || result.Processing[0].Version == nil || *result.Processing[0].Version != "production-test" {
		t.Fatal("JSON не содержит сведения об обработанном файле и текущем запуске")
	}
	if result.Document.DSLVersion == nil || *result.Document.DSLVersion != "1.2" || result.Document.SHA256 == nil || *result.Document.SHA256 != fmt.Sprintf("%x", sha256.Sum256(source)) || result.Document.ByteLength == nil || *result.Document.ByteLength != int64(len(source)) || result.Document.LineCount == nil || *result.Document.LineCount != bytes.Count(source, []byte{'\n'}) || len(result.Lines) != *result.Document.LineCount {
		t.Fatal("Версия, байтовые факты или число строк JSON не соответствуют полной копии")
	}
	var wire struct {
		Document struct{ Metadata map[string]json.RawMessage }
		Lines    []struct{ Elements []map[string]json.RawMessage }
	}
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if value, present := wire.Document.Metadata["order"]; !present || (result.Document.Metadata.Order == nil && string(value) != "null") {
		t.Fatal("metadata.order должен присутствовать, включая явный null")
	}
	want = slices.Clone(want)
	sort.SliceStable(want, func(i, j int) bool {
		if want[i].line != want[j].line {
			return want[i].line < want[j].line
		}
		return want[i].start < want[j].start
	})
	if len(result.Diagnostics) != len(want) || result.Diagnostics == nil || result.Document.HasErrors != (len(want) > 0) {
		t.Fatalf("Диагностики JSON %+v, требуются %+v", result.Diagnostics, want)
	}
	byID := make(map[string]model.Diagnostic)
	badLines := make(map[int]bool)
	for i, expected := range want {
		d := result.Diagnostics[i]
		location := model.Location{Start: model.Position{Line: expected.line, Column: expected.start}, End: model.Position{Line: expected.line, Column: expected.end}}
		if _, duplicate := byID[d.ID]; duplicate || d.ID == "" || d.Source != result.Processing[0].ID || d.DiagnosticCode != expected.code || d.Fatal || d.SeverityLevel != diagnostics.SeverityError || d.DiagnosticScope != diagnostics.ScopeElement || d.Location == nil || *d.Location != location {
			t.Fatalf("Неверная диагностика %+v, требуется %+v", d, expected)
		}
		matches := 0
		for _, e := range result.Lines[expected.line-1].Elements {
			if e.ElementType == model.ElementTypeUnparsed && e.Raw == expected.raw && e.Value == nil && e.Start == expected.start && e.End == expected.end && slices.Equal(e.ErrorIDs, []string{d.ID}) {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("Нет единственного unparsed с точной ссылкой для %+v", expected)
		}
		byID[d.ID], badLines[expected.line] = d, true
	}
	var restored strings.Builder
	links := make(map[string]int)
	for i, line := range result.Lines {
		if line.Number != i+1 || line.HasErrors != badLines[line.Number] || line.Elements == nil {
			t.Fatalf("Неверный номер, hasErrors или elements строки: %+v", line)
		}
		if line.ParentLine == nil {
			if line.NestingLevel != 0 {
				t.Fatal("У корневой строки ненулевая вложенность")
			}
		} else if *line.ParentLine < 1 || *line.ParentLine >= line.Number || line.NestingLevel != result.Lines[*line.ParentLine-1].NestingLevel+1 {
			t.Fatalf("Неверная связь родителя: %+v", line)
		}
		restored.WriteString(line.Raw)
		restored.WriteString(string(line.LineEnding))
		runes, previousEnd := []rune(line.Raw), 1
		for j, e := range line.Elements {
			if e.Start < previousEnd || e.End < e.Start || e.End > len(runes)+1 || string(runes[e.Start-1:e.End-1]) != e.Raw || e.ErrorIDs == nil {
				t.Fatalf("Нарушен Unicode-диапазон, raw или errorIds: %+v", e)
			}
			previousEnd = e.End
			if e.ElementType == model.ElementTypeUnparsed && (e.Value != nil || len(e.ErrorIDs) == 0 || string(wire.Lines[i].Elements[j]["value"]) != "null") {
				t.Fatal("unparsed должен иметь явный value:null и диагностическую ссылку")
			}
			seen := make(map[string]bool)
			for _, id := range e.ErrorIDs {
				d, exists := byID[id]
				location := model.Location{Start: model.Position{Line: line.Number, Column: e.Start}, End: model.Position{Line: line.Number, Column: e.End}}
				if !exists || seen[id] || d.Location == nil || *d.Location != location {
					t.Fatalf("Повторная, потерянная или неточная ссылка %q", id)
				}
				seen[id], links[id] = true, links[id]+1
			}
		}
	}
	for id := range byID {
		if links[id] != 1 {
			t.Fatalf("Диагностика %s связана с %d элементами", id, links[id])
		}
	}
	unchanged, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(unchanged, source) || restored.String() != string(source) {
		t.Fatal("Пропущены последующие строки или изменён исходный текст")
	}
	return result
}

func assertProductionElements(t *testing.T, line model.Line, kind model.LineType, want ...productionElement) {
	t.Helper()
	if line.LineType != kind || len(line.Elements) != len(want) {
		t.Fatalf("Неверный тип строки или набор элементов: %+v", line)
	}
	for i, expected := range want {
		e := line.Elements[i]
		if e.ElementType != expected.kind || e.Raw != expected.raw || e.Start != expected.start || e.End != expected.start+utf8.RuneCountInString(expected.raw) {
			t.Fatalf("Элемент %+v, требуется %+v", e, expected)
		}
		if expected.kind == model.ElementTypeUnparsed {
			if e.Value != nil || len(e.ErrorIDs) != 1 {
				t.Fatal("Неверное представление ошибочного элемента")
			}
		} else if e.Value == nil || *e.Value != expected.value || len(e.ErrorIDs) != 0 {
			t.Fatalf("Изменено корректное значение или соседний элемент: %+v", e)
		}
	}
}

// Сравниваются все неизменённые строки, включая структуру и последующие строки.
// ID могут сдвигаться после новой ошибки; их точность отдельно проверена выше.
func assertProductionUnchanged(t *testing.T, base, result model.Result, insertedAt, count int, except ...int) {
	t.Helper()
	shift := func(number int) int {
		if count > 0 && number >= insertedAt {
			return number + count
		}
		return number
	}
	stripIDs := func(line model.Line) model.Line {
		line.Elements = slices.Clone(line.Elements)
		for i := range line.Elements {
			line.Elements[i].ErrorIDs = nil
		}
		return line
	}
	for _, line := range base.Lines {
		if slices.Contains(except, line.Number) {
			continue
		}
		line.Number = shift(line.Number)
		if line.ParentLine != nil {
			parent := shift(*line.ParentLine)
			line.ParentLine = &parent
		}
		if !reflect.DeepEqual(stripIDs(line), stripIDs(result.Lines[line.Number-1])) {
			t.Fatalf("Изменён разбор соседней/последующей строки %d: получено %+v, требуется %+v", line.Number, result.Lines[line.Number-1], line)
		}
	}
}

func assertProductionMetadata(t *testing.T, result model.Result, want model.DocumentMetadata) {
	t.Helper()
	if !reflect.DeepEqual(result.Document.Metadata, want) {
		t.Fatalf("Метаданные %+v, требуются %+v", result.Document.Metadata, want)
	}
}
