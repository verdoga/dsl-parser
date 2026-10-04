package diagnostics

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestCheckP001DetectsInvalidUTF8(t *testing.T) {
	tests := []struct {
		name, source string
		matched      bool
	}{
		{name: "empty"},
		{name: "ASCII", source: "@dsl-version 1.2\n"},
		{name: "multibyte", source: "Привет, 世界 🌍"},
		{name: "BOM", source: "\uFEFF@dsl-version 1.2"},
		{name: "replacement character", source: "\uFFFD"},
		{name: "invalid byte", source: "text\xff", matched: true},
		{name: "isolated continuation", source: "\x80", matched: true},
		{name: "truncated sequence", source: "\xe2\x82", matched: true},
		{name: "overlong encoding", source: "\xc0\xaf", matched: true},
		{name: "surrogate", source: "\xed\xa0\x80", matched: true},
		{name: "above Unicode range", source: "\xf4\x90\x80\x80", matched: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertCheckResult(t, P001, checkP001, test.source, test.matched)
		})
	}
}

func TestCheckP002DetectsBareCR(t *testing.T) {
	tests := []struct {
		name, source string
		matched      bool
	}{
		{name: "empty"},
		{name: "without line ending", source: "text"},
		{name: "LF", source: "one\ntwo\n"},
		{name: "CRLF", source: "one\r\ntwo\r\n"},
		{name: "mixed endings", source: "one\ntwo\r\nthree"},
		{name: "CR alone", source: "\r", matched: true},
		{name: "CR inside", source: "one\rtwo", matched: true},
		{name: "CR at end", source: "one\r", matched: true},
		{name: "CR before CRLF", source: "one\r\r\n", matched: true},
		{name: "CR after LF", source: "one\n\r", matched: true},
		{name: "independent of UTF8", source: "\xff\r\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertCheckResult(t, P002, checkP002, test.source, test.matched)
		})
	}
}

func TestVersionChecksParseFirstPhysicalLine(t *testing.T) {
	tests := []struct {
		name, source string
		p013, p014   bool
	}{
		{name: "supported", source: "@dsl-version 1.2"},
		{name: "unsupported", source: "@dsl-version 2.0", p014: true},
		{name: "version spelling is exact", source: "@dsl-version 1.20", p014: true},
		{name: "other version token", source: "@dsl-version future", p014: true},
		{name: "uppercase tag", source: "@DSL-VERSION 1.2"},
		{name: "mixed case tag", source: "@DsL-Version 2.0", p014: true},
		{name: "edge whitespace", source: " \t@dsl-version 1.2\t "},
		{name: "multiple separator spaces", source: "@dsl-version   1.2"},
		{name: "initial BOM", source: "\uFEFF@dsl-version 1.2"},
		{name: "BOM before indentation", source: "\uFEFF \t@dsl-version 1.2"},
		{name: "LF", source: "@dsl-version 1.2\n"},
		{name: "CRLF", source: "@dsl-version 1.2\r\n"},
		{name: "CRLF with trailing whitespace", source: "@dsl-version 1.2 \t\r\n"},
		{name: "unsupported with BOM and CRLF", source: "\uFEFF@dsl-version 2.0\r\n", p014: true},
		{name: "empty", p013: true},
		{name: "BOM only", source: "\uFEFF", p013: true},
		{name: "whitespace only", source: " \t", p013: true},
		{name: "missing tag", source: "# Title", p013: true},
		{name: "tag on second line", source: "\n@dsl-version 1.2", p013: true},
		{name: "comment before tag", source: "@editor comment\n@dsl-version 1.2", p013: true},
		{name: "missing version", source: "@dsl-version", p013: true},
		{name: "whitespace instead of version", source: "@dsl-version \t ", p013: true},
		{name: "missing separator", source: "@dsl-version1.2", p013: true},
		{name: "tab separator", source: "@dsl-version\t1.2", p013: true},
		{name: "space then tab separator", source: "@dsl-version \t1.2", p013: true},
		{name: "tab then space separator", source: "@dsl-version\t 1.2", p013: true},
		{name: "extra parameter", source: "@dsl-version 1.2 extra", p013: true},
		{name: "unsupported with extra parameter", source: "@dsl-version 2.0 extra", p013: true},
		{name: "tab inside version", source: "@dsl-version 1.\t2", p013: true},
		{name: "Unicode space inside version", source: "@dsl-version 1.\u00a02", p013: true},
		{name: "wrong tag", source: "@dsl-version-extra 1.2", p013: true},
		{name: "escaped tag", source: "\\@dsl-version 1.2", p013: true},
		{name: "two initial BOMs", source: "\uFEFF\uFEFF@dsl-version 1.2", p013: true},
		{name: "BOM after space", source: " \uFEFF@dsl-version 1.2", p013: true},
		{name: "bare CR at end", source: "@dsl-version 1.2\r", p013: true},
		{name: "bare CR inside first line", source: "@dsl-version 1.2\rtext\n", p013: true},
		{name: "repeated supported declaration", source: "@dsl-version 1.2\n@dsl-version 1.2"},
		{name: "later unsupported declaration", source: "@dsl-version 1.2\n@dsl-version 2.0"},
		{name: "later supported declaration", source: "@dsl-version 2.0\n@dsl-version 1.2", p014: true},
		{name: "invalid UTF8 after first line", source: "@dsl-version 1.2\n\xff"},
		{name: "bare CR after first line", source: "@dsl-version 1.2\ntext\rtail"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("P013", func(t *testing.T) { assertCheckResult(t, P013, checkP013, test.source, test.p013) })
			t.Run("P014", func(t *testing.T) { assertCheckResult(t, P014, checkP014, test.source, test.p014) })
		})
	}
}

func TestChecksReadEntireStreamAndPreserveErrors(t *testing.T) {
	readErr := errors.New("ошибка чтения исходника")
	checks := []struct {
		code  Code
		check CheckFunc
	}{{IO001, checkIO001}, {P001, checkP001}, {P002, checkP002}, {P013, checkP013}, {P014, checkP014}}
	tests := []struct {
		name, source string
		endErr       error
		withData     bool
		chunkSize    int
		matched      map[Code]bool
	}{
		{name: "empty stream", endErr: io.EOF, matched: map[Code]bool{P013: true}},
		{name: "complete stream", source: "@dsl-version 1.2\nПривет", endErr: io.EOF},
		{name: "match before end", source: "\xff\rtail", endErr: io.EOF, chunkSize: 1, matched: map[Code]bool{P001: true, P002: true, P013: true}},
		{name: "unsupported before end", source: "@dsl-version 2.0\ntail", endErr: io.EOF, chunkSize: 1, matched: map[Code]bool{P014: true}},
		{name: "single byte reads", source: "\uFEFF\t@DsL-Version  1.2 \t\r\nПривет\r\n", endErr: io.EOF, chunkSize: 1},
		{name: "data and EOF", source: "\xff\rtail", endErr: io.EOF, withData: true, matched: map[Code]bool{P001: true, P002: true, P013: true}},
		{name: "error before data", endErr: readErr, matched: map[Code]bool{IO001: true}},
		{name: "error after partial data", source: "\xff\rtail", endErr: readErr, chunkSize: 1, matched: map[Code]bool{IO001: true}},
		{name: "data and error together", source: "\xff\rtail", endErr: readErr, withData: true, matched: map[Code]bool{IO001: true}},
		{name: "error after supported version", source: "@dsl-version 1.2\ntail", endErr: readErr, matched: map[Code]bool{IO001: true}},
		{name: "error after unsupported version", source: "@dsl-version 2.0\ntail", endErr: readErr, matched: map[Code]bool{IO001: true}},
	}
	for _, check := range checks {
		t.Run(string(check.code), func(t *testing.T) {
			description, found := NewRegistry().Lookup(check.code)
			if !found || description == nil {
				t.Fatal("реестр не вернул описание проверки")
			}
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					for _, call := range []struct {
						name  string
						check CheckFunc
					}{{"direct", check.check}, {"registered", description.Check}} {
						t.Run(call.name, func(t *testing.T) {
							reader := &checkStreamReader{source: test.source, endErr: test.endErr, withData: test.withData, chunkSize: test.chunkSize}
							matched, err := call.check(reader)
							wantErr := test.endErr
							if wantErr == io.EOF {
								wantErr = nil
							}
							if err != wantErr {
								t.Errorf("ошибка = %v, требуется исходная ошибка %v", err, wantErr)
							}
							if matched != test.matched[check.code] {
								t.Errorf("matched = %t, требуется %t", matched, test.matched[check.code])
							}
							if !reader.ended || reader.source != "" {
								t.Error("проверка не дочитала поток до конца или технической ошибки")
							}
							if reader.closed || reader.sought {
								t.Errorf("проверка закрыла или перемотала поток: closed=%t, sought=%t", reader.closed, reader.sought)
							}
						})
					}
				})
			}
		})
	}
}

func assertCheckResult(t *testing.T, code Code, check CheckFunc, source string, want bool) {
	t.Helper()
	description, found := NewRegistry().Lookup(code)
	if !found || description == nil {
		t.Fatalf("Lookup(%q) не вернул описание", code)
	}
	for _, call := range []struct {
		name  string
		check CheckFunc
	}{{"direct", check}, {"registered", description.Check}} {
		t.Run(call.name, func(t *testing.T) {
			matched, err := call.check(strings.NewReader(source))
			if err != nil {
				t.Fatalf("проверка вернула техническую ошибку: %v", err)
			}
			if matched != want {
				t.Errorf("matched = %t, требуется %t", matched, want)
			}
		})
	}
}

type checkStreamReader struct {
	source                string
	endErr                error
	withData              bool
	chunkSize             int
	ended, closed, sought bool
}

func (r *checkStreamReader) Read(p []byte) (int, error) {
	if r.source == "" {
		r.ended = true
		return 0, r.endErr
	}
	if r.chunkSize > 0 && len(p) > r.chunkSize {
		p = p[:r.chunkSize]
	}
	n := copy(p, r.source)
	r.source = r.source[n:]
	if r.source == "" && r.withData {
		r.ended = true
		return n, r.endErr
	}
	return n, nil
}

func (r *checkStreamReader) Close() error {
	r.closed = true
	return nil
}

func (r *checkStreamReader) Seek(int64, int) (int64, error) {
	r.sought = true
	return 0, nil
}
