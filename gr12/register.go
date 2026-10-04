package gr12

import "github.com/verdoga/dsl-parser/grammar"

// BuiltinGr12 регистрирует функции разбора и два утверждения DSL v1.2
// в реестре grammar; состояние между строками пакет не хранит.
func BuiltinGr12(registry grammar.Registry) {
	registry.Register(
		"1.2",
		[]grammar.DetectionFunc{detectLine},
		[]grammar.OrchestrationFunc{orchestrateLine},
		[]grammar.LineTypeFunc{classifyLine},
		[]grammar.LineParserFunc{parseLine},
		[]grammar.Assertion{
			{ID: IncreasesNesting, Check: increasesNesting},
			{ID: DecreasesNesting, Check: decreasesNesting},
		},
	)
}
