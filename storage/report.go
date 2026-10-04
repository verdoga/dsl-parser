// Package storage сохраняет готовую модель результата парсера в JSON рядом
// с исходным TXT-файлом. Пакет проверяет право замены и фатальные диагностики,
// сериализует модель и возвращает отчёт для консольного reporter.
// Обход каталогов, разбор DSL и сопоставление разных JSON в пакет не входят.
package storage

// Action — фактическое действие с целевым JSON-файлом.
type Action string

const (
	// ActionNotWritten — новый JSON не установлен.
	ActionNotWritten Action = "not-written"

	// ActionCreated — новый JSON создан.
	ActionCreated Action = "created"

	// ActionReplaced — целевой JSON заменён новым.
	ActionReplaced Action = "replaced"
)

// Report — результат одной попытки записи, полностью передаваемый reporter.
// Err равен nil только при успешном завершении операции.
type Report struct {
	// SourcePath — абсолютный очищенный путь исходного TXT или пустая строка,
	// если путь нельзя получить из модели.
	SourcePath string

	// TargetPath — путь целевого JSON или пустая строка, если он не вычислен.
	TargetPath string

	// Action — фактически выполненное действие с целевым JSON.
	Action Action

	// ErrorCount — число диагностик с severity="error" в переданной модели.
	ErrorCount int

	// Err — причина отказа или сбоя записи; при фатальных диагностиках содержит
	// их коды и сообщения в порядке модели. Поле не записывается в JSON.
	Err error `json:"-"`
}

// NewReport создаёт отчёт без обращения к файловой системе.
// Значения action и err отражают фактический итог попытки записи.
func NewReport(sourcePath, targetPath string, action Action, errorCount int, err error) Report {
	return Report{
		SourcePath: sourcePath,
		TargetPath: targetPath,
		Action:     action,
		ErrorCount: errorCount,
		Err:        err,
	}
}
