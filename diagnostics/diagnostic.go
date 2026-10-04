package diagnostics

import (
	"errors"
	"io"
)

// ErrCheckUnavailable возвращается методом Check, если для диагностики
// не зарегистрирована функция проверки.
var ErrCheckUnavailable = errors.New("диагностическая функция не зарегистрирована")

// CheckFunc проверяет поток исходных данных.
// matched равен true при обнаружении условия диагностики, false — при его отсутствии.
// err описывает техническую ошибку проверки.
type CheckFunc func(io.Reader) (matched bool, err error)

// Diagnostic предоставляет доступ только для чтения к описанию диагностики.
type Diagnostic interface {
	// Code возвращает машинный код диагностики.
	Code() Code

	// Severity возвращает уровень серьёзности диагностики.
	Severity() Severity

	// Message возвращает нормативное человекочитаемое описание диагностики.
	Message() string

	// Scope возвращает область диагностики по JSON-контракту.
	Scope() Scope

	// IsFatal сообщает, прекращает ли диагностика штатный разбор.
	IsFatal() bool

	// HasCheck сообщает, зарегистрирована ли функция проверки.
	HasCheck() bool

	// Check вызывает функцию проверки.
	// При её отсутствии возвращает false и ErrCheckUnavailable.
	Check(reader io.Reader) (matched bool, err error)
}

// diagnostic хранит неизменяемое внутреннее описание одной диагностики.
type diagnostic struct {
	// code — устойчивый машинный код диагностики.
	code Code

	// severity — уровень серьёзности.
	severity Severity

	// message — нормативное описание диагностики из реестра.
	message string

	// scope — область диагностики в результате разбора.
	scope Scope

	// fatal — признак невозможности продолжить штатный разбор.
	fatal bool

	// check — необязательная функция обнаружения условия диагностики; может быть nil.
	check CheckFunc
}

// Code возвращает код описания диагностики.
func (d diagnostic) Code() Code {
	return d.code
}

// Severity возвращает уровень серьёзности описания диагностики.
func (d diagnostic) Severity() Severity {
	return d.severity
}

// Message возвращает нормативное описание диагностики.
func (d diagnostic) Message() string {
	return d.message
}

// Scope возвращает область описания диагностики.
func (d diagnostic) Scope() Scope {
	return d.scope
}

// IsFatal сообщает значение признака fatal.
func (d diagnostic) IsFatal() bool {
	return d.fatal
}

// HasCheck сообщает о наличии диагностической функции.
func (d diagnostic) HasCheck() bool {
	return d.check != nil
}

// Check вызывает зарегистрированную диагностическую функцию.
func (d diagnostic) Check(reader io.Reader) (matched bool, err error) {
	if d.check == nil {
		return false, ErrCheckUnavailable
	}
	return d.check(reader)
}
