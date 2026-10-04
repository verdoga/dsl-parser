package gr12

import "github.com/verdoga/dsl-parser/grammar"

// BuiltinGr12 регистрирует функции разбора DSL v1.2 и ID утверждений об открытии
// и завершении логического родителя. Функции утверждений возвращаются отдельно
// в новой карте, принадлежащей вызывающему коду, и в реестре не хранятся.
// Повторный вызов заменяет регистрацию версии 1.2 и возвращает независимую карту;
// состояние между строками пакет не хранит.
func BuiltinGr12(registry grammar.Registry) map[grammar.AssertionID]grammar.AssertionFunc {
	registry.Register(
		"1.2",
		[]grammar.DetectionFunc{detectLine},
		[]grammar.OrchestrationFunc{orchestrateLine},
		[]grammar.LineTypeFunc{classifyLine},
		[]grammar.LineParserFunc{parseLine},
		IncreasesNesting,
		DecreasesNesting,
	)
	return map[grammar.AssertionID]grammar.AssertionFunc{
		IncreasesNesting: increasesNesting,
		DecreasesNesting: decreasesNesting,
	}
}
