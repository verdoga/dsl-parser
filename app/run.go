// Package app оркестрирует один запуск dslparser: принимает аргументы,
// находит TXT-файлы, готовит реестры и последовательно передаёт каждый
// источник парсеру, хранилищу и консольному отчётчику. Пакет создаёт начальную
// модель, задаёт время её обработки и выбирает код завершения. Сам пакет не
// разбирает DSL, не обходит каталоги и не записывает JSON.
package app

import (
	"fmt"
	"os"

	"github.com/verdoga/dsl-parser/console"
	"github.com/verdoga/dsl-parser/diagnostics"
	"github.com/verdoga/dsl-parser/discovery"
	"github.com/verdoga/dsl-parser/reporter"
)

const (
	// exitSuccess — обработка завершилась без ошибок файлов и обхода.
	exitSuccess = 0

	// exitFailure — ошибка файла, обхода либо подготовки конвейера.
	exitFailure = 1

	// exitUsage — ошибка аргументов или исходного пути до поиска файлов.
	exitUsage = 2

	// toolName — стабильное машинное имя инструмента в model.Processing.
	toolName = "dsl-parser"

	// processingID — ID единственного запуска инструмента в новой модели;
	// идентификаторы разных результатов не обязаны отличаться друг от друга.
	processingID = "p1"
)

// Run получает аргументы без имени программы и непустую версию инструмента,
// заданную при сборке. Передаёт args в console.Parse с os.Stdout и os.Stderr;
// при proceed=false возвращает 0 для справки либо 2 для ошибки без повторной
// печати. Пустую версию считает ошибкой конфигурации: печатает её и возвращает
// 1. После проверки версии вызывает discovery.Find. Ошибку обхода
// печатает в os.Stderr, сохраняя возвращённые отсортированные пути для обработки.
// Создаёт реестры, затем reporter.New и последовательно вызывает processFile.
// При ошибке подготовки реестров печатает причину и возвращает 1 без обработки.
// После цикла вызывает reporter.Finish ровно один раз и возвращает его код,
// повышая 0 до 1 при ошибке discovery. Ноль найденных TXT без ошибки допустим.
// Run не вызывает os.Exit: возвращённый код передаёт ему пакет main.
func Run(args []string, version string) int {
	params, proceed, err := console.Parse(args, os.Stdout, os.Stderr)
	if !proceed {
		if err != nil {
			return exitUsage
		}
		return exitSuccess
	}
	if version == "" {
		fmt.Fprintln(os.Stderr, "Ошибка конфигурации: версия инструмента пуста")
		return exitFailure
	}
	paths, discoveryErr := discovery.Find(params)
	if discoveryErr != nil {
		fmt.Fprintf(os.Stderr, "Ошибка поиска файлов: %v\n", discoveryErr)
	}
	grammars, assertions, err := prepareGrammar()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка подготовки реестров: %v\n", err)
		return exitFailure
	}
	diagnosticRegistry := diagnostics.NewRegistry()
	r := runner{
		params: params, version: version, grammars: grammars,
		diagnostics: diagnosticRegistry, assertions: assertions, reporter: reporter.New(),
	}
	for _, path := range paths {
		r.processFile(path)
	}
	code := r.reporter.Finish()
	if code == exitSuccess && discoveryErr != nil {
		return exitFailure
	}
	return code
}
