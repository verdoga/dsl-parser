package parser

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/model"
)

// prepareInput выполняет байтовый этап. После полного чтения вызывает обе
// проверки на независимых читателях. При ранней диагностике возвращает ready=false,
// оставляя Lines=[] и LineCount=nil. При успехе возвращает точные Raw и EOL.
func (p *Parser) prepareInput() (lines []model.Line, ready bool, err error) {
	document := &p.result.Document
	p.result.Lines = []model.Line{}
	document.LineCount, document.Encoding, document.DSLVersion = nil, nil, nil
	document.ByteLength, document.SHA256, document.HasBOM = nil, nil, nil
	source, ok, err := p.readBytes()
	if err != nil || !ok {
		return nil, false, err
	}
	recordByteFacts(document, source)
	invalidUTF8, utf8Err := p.checkUTF8(source)
	invalidEndings, endingsErr := p.checkLineEndings(source)
	if err := errors.Join(utf8Err, endingsErr); err != nil {
		return nil, false, err
	}
	if invalidUTF8 || invalidEndings {
		return nil, false, nil
	}
	encoding := "UTF-8"
	document.Encoding = &encoding
	lines = splitLines(source)
	version, ok, err := p.readVersion(lines)
	if err != nil || !ok {
		return nil, false, err
	}
	p.version, document.DSLVersion = version, &version
	p.seenMetadata["@dsl-version"] = 1
	if ok, err := p.selectGrammar(); err != nil || !ok {
		return nil, false, err
	}
	count := len(lines)
	document.LineCount = &count
	return lines, true, nil
}

// readBytes получает полный источник через зарегистрированную проверку IO001.
// Ошибка чтения становится диагностикой; неполные байты не возвращаются как
// полный источник. Отсутствующая проверка означает ошибку конфигурации.
func (p *Parser) readBytes() (source []byte, ok bool, err error) {
	if p.reader == nil || p.diagnostics == nil {
		return nil, false, fmt.Errorf("не задан поток или реестр диагностик")
	}
	description, found := p.diagnostics.Lookup(diagnostics.IO001)
	if !found || description == nil || !description.HasCheck() {
		return nil, false, fmt.Errorf("IO001: %w", diagnostics.ErrCheckUnavailable)
	}
	var buffer bytes.Buffer
	matched, readErr := description.Check(io.TeeReader(p.reader, &buffer))
	if !matched && readErr == nil {
		return buffer.Bytes(), true, nil
	}
	occurrence := model.Diagnostic{DiagnosticCode: diagnostics.IO001}
	if readErr != nil {
		occurrence.Message = fmt.Sprintf("%s: %v", description.Message(), readErr)
	}
	diagnostic, err := p.completeDiagnostic(occurrence)
	if err != nil {
		return nil, false, err
	}
	p.appendDiagnostic(diagnostic)
	return nil, false, nil
}

// recordByteFacts заполняет ByteLength, SHA256 и HasBOM по точным байтам,
// включая начальный BOM и исходные переводы строк.
func recordByteFacts(document *model.Document, source []byte) {
	length := int64(len(source))
	hash := fmt.Sprintf("%x", sha256.Sum256(source))
	hasBOM := bytes.HasPrefix(source, []byte("\uFEFF"))
	document.ByteLength, document.SHA256, document.HasBOM = &length, &hash, &hasBOM
}

// checkUTF8 вызывает зарегистрированную проверку P001 на точных байтах.
// matched означает раннюю диагностику; отсутствие проверки — ошибку конфигурации.
func (p *Parser) checkUTF8(source []byte) (matched bool, err error) {
	if p.diagnostics == nil {
		return false, fmt.Errorf("не задан реестр диагностик")
	}
	description, found := p.diagnostics.Lookup(diagnostics.P001)
	if !found || description == nil || !description.HasCheck() {
		return false, fmt.Errorf("P001: %w", diagnostics.ErrCheckUnavailable)
	}
	matched, err = description.Check(bytes.NewReader(source))
	if err != nil {
		return false, fmt.Errorf("проверка P001: %w", err)
	}
	if matched {
		diagnostic, err := p.completeDiagnostic(model.Diagnostic{DiagnosticCode: diagnostics.P001})
		if err != nil {
			return true, err
		}
		p.appendDiagnostic(diagnostic)
	}
	return matched, nil
}

// checkLineEndings вызывает проверку P002 на новом читателе тех же байтов,
// независимо от P001. Отсутствующая проверка означает ошибку конфигурации.
func (p *Parser) checkLineEndings(source []byte) (matched bool, err error) {
	if p.diagnostics == nil {
		return false, fmt.Errorf("не задан реестр диагностик")
	}
	description, found := p.diagnostics.Lookup(diagnostics.P002)
	if !found || description == nil || !description.HasCheck() {
		return false, fmt.Errorf("P002: %w", diagnostics.ErrCheckUnavailable)
	}
	matched, err = description.Check(bytes.NewReader(source))
	if err != nil {
		return false, fmt.Errorf("проверка P002: %w", err)
	}
	if matched {
		diagnostic, err := p.completeDiagnostic(model.Diagnostic{DiagnosticCode: diagnostics.P002})
		if err != nil {
			return true, err
		}
		p.appendDiagnostic(diagnostic)
	}
	return matched, nil
}

// splitLines удаляет только начальный UTF-8 BOM и выделяет точные Raw и EOL.
// Завершающий перевод строки не создаёт фиктивную дополнительную строку.
func splitLines(source []byte) []model.Line {
	source = bytes.TrimPrefix(source, []byte("\uFEFF"))
	lines := []model.Line{}
	for len(source) > 0 {
		raw, rest, hasLF := bytes.Cut(source, []byte{'\n'})
		ending := model.LineEndingNone
		if hasLF {
			ending = model.LineEndingLF
			if bytes.HasSuffix(raw, []byte{'\r'}) {
				raw = raw[:len(raw)-1]
				ending = model.LineEndingCRLF
			}
		}
		lines = append(lines, model.Line{Number: len(lines) + 1, Raw: string(raw), LineEnding: ending})
		source = rest
	}
	return lines
}

// readVersion проверяет форму первой строки через P013 без знания грамматики.
// P013 завершает подготовку, в том числе при пустом документе.
func (p *Parser) readVersion(lines []model.Line) (version string, ok bool, err error) {
	if p.diagnostics == nil {
		return "", false, fmt.Errorf("не задан реестр диагностик")
	}
	description, found := p.diagnostics.Lookup(diagnostics.P013)
	if !found || description == nil || !description.HasCheck() {
		return "", false, fmt.Errorf("P013: %w", diagnostics.ErrCheckUnavailable)
	}
	first := ""
	if len(lines) > 0 {
		first = lines[0].Raw + string(lines[0].LineEnding)
	}
	// Восстанавливаем удалённый splitLines BOM для проверки исходной формы:
	// два начальных BOM не должны превратиться в допустимое объявление.
	if p.result.Document.HasBOM != nil && *p.result.Document.HasBOM {
		first = "\uFEFF" + first
	}
	matched, err := description.Check(strings.NewReader(first))
	if err != nil {
		return "", false, fmt.Errorf("проверка P013: %w", err)
	}
	if matched {
		diagnostic, err := p.completeDiagnostic(model.Diagnostic{DiagnosticCode: diagnostics.P013})
		if err != nil {
			return "", false, err
		}
		p.appendDiagnostic(diagnostic)
		return "", false, nil
	}
	if len(lines) == 0 {
		return "", false, fmt.Errorf("проверка P013 приняла пустой документ")
	}
	_, version, _ = strings.Cut(strings.Trim(lines[0].Raw, " \t"), " ")
	return strings.TrimLeft(version, " "), true, nil
}

// selectGrammar выдаёт P014 при отсутствии версии в реестре. Иначе выбирает
// функции грамматики, ролевые ID и переданную карту утверждений выбранной версии.
// Отсутствие обязательного ID или ненулевой функции — ошибка конфигурации.
func (p *Parser) selectGrammar() (ok bool, err error) {
	selected, found := p.grammars.Lookup(p.version)
	if !found {
		diagnostic, err := p.completeDiagnostic(model.Diagnostic{DiagnosticCode: diagnostics.P014})
		if err != nil {
			return false, err
		}
		p.appendDiagnostic(diagnostic)
		return false, nil
	}
	enter, leave := selected.Assertions()
	assertions := p.assertionFuncs[p.version]
	if assertions == nil || enter == "" || leave == "" || assertions[enter] == nil || assertions[leave] == nil {
		return false, fmt.Errorf("грамматика %q: не заданы структурные роли или функции утверждений", p.version)
	}
	p.detections, p.orchestrators = selected.DetectionFuncs(), selected.OrchestrationFuncs()
	p.enterParentID, p.leaveParentID, p.assertions = enter, leave, assertions
	return true, nil
}
