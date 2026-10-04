package diagnostics

import (
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// checkIO001 читает поток до конца и возвращает true с исходной ошибкой чтения
// либо false, nil при успешном чтении. Поток не закрывается и не перематывается.
func checkIO001(reader io.Reader) (matched bool, err error) {
	_, err = io.Copy(io.Discard, reader)
	return err != nil, err
}

// checkP001 читает поток до конца и возвращает true для некорректного UTF-8,
// false — для корректного. При ошибке чтения возвращает false и исходную ошибку.
// Поток не закрывается и не перематывается.
func checkP001(reader io.Reader) (matched bool, err error) {
	source, err := io.ReadAll(reader)
	if err != nil {
		return false, err
	}
	return !utf8.Valid(source), nil
}

// checkP002 читает поток до конца и возвращает true при наличии CR вне CRLF,
// false — при его отсутствии. При ошибке чтения возвращает false и исходную ошибку.
// Поток не закрывается и не перематывается.
func checkP002(reader io.Reader) (matched bool, err error) {
	source, err := io.ReadAll(reader)
	if err != nil {
		return false, err
	}
	for i, b := range source {
		if b == '\r' && (i+1 == len(source) || source[i+1] != '\n') {
			return true, nil
		}
	}
	return false, nil
}

// checkP013 читает поток до конца и возвращает true, если первая строка после
// удаления одного начального BOM не содержит корректного объявления версии.
// Тег регистронезависим; края строки очищаются от пробелов и табуляций,
// разделитель состоит из U+0020, версия — один непустой токен без пробельных символов.
// Корректное объявление даёт false; поддержка версии и повторные объявления не проверяются.
// При ошибке чтения возвращает false и исходную ошибку.
// Поток не закрывается и не перематывается.
func checkP013(reader io.Reader) (matched bool, err error) {
	source, err := io.ReadAll(reader)
	if err != nil {
		return false, err
	}
	line, _, hasLF := strings.Cut(strings.TrimPrefix(string(source), "\uFEFF"), "\n")
	if hasLF {
		line = strings.TrimSuffix(line, "\r")
	}
	line = strings.Trim(line, " \t")
	tag, version, separated := strings.Cut(line, " ")
	if !separated || strings.ToLower(tag) != "@dsl-version" {
		return true, nil
	}
	version = strings.TrimLeft(version, " ")
	if version == "" || strings.IndexFunc(version, unicode.IsSpace) >= 0 {
		return true, nil
	}
	return false, nil
}

// checkP014 читает поток до конца и возвращает true, если корректное объявление
// в первой строке задаёт версию, отличную от точного значения "1.2".
// Один начальный BOM удаляется; тег регистронезависим, края строки очищаются
// от пробелов и табуляций, разделитель состоит из U+0020, версия не содержит пробельных символов.
// Поддерживаемая версия или некорректное объявление дают false; повторы не проверяются.
// При ошибке чтения возвращает false и исходную ошибку.
// Поток не закрывается и не перематывается.
func checkP014(reader io.Reader) (matched bool, err error) {
	source, err := io.ReadAll(reader)
	if err != nil {
		return false, err
	}
	line, _, hasLF := strings.Cut(strings.TrimPrefix(string(source), "\uFEFF"), "\n")
	if hasLF {
		line = strings.TrimSuffix(line, "\r")
	}
	line = strings.Trim(line, " \t")
	tag, version, separated := strings.Cut(line, " ")
	if !separated || strings.ToLower(tag) != "@dsl-version" {
		return false, nil
	}
	version = strings.TrimLeft(version, " ")
	if version == "" || strings.IndexFunc(version, unicode.IsSpace) >= 0 {
		return false, nil
	}
	return version != "1.2", nil
}
