package main

import (
	"os"

	"github.com/verdoga/dsl-parser/app"
)

// version — версия приложения, записываемая в результат обработки.
const version = "1.0.0"

// main запускает обработку аргументов и завершает процесс с кодом приложения.
func main() {
	os.Exit(app.Run(os.Args[1:], version))
}
