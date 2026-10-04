package app

import (
	"fmt"

	"github.com/verdoga/dsl-parser/gr12"
	"github.com/verdoga/dsl-parser/grammar"
)

// supportedDSLVersion — версия грамматики, регистрируемой пакетом gr12.
const supportedDSLVersion = "1.2"

// prepareGrammar создаёт grammar.NewRegistry, вызывает gr12.BuiltinGr12 и
// сохраняет возвращённые функции проверок. Через Lookup для supportedDSLVersion
// и Assertions получает структурные роли и собирает карту для parser.New.
// Проверяет роли и наличие ненулевых проверок для gr12.IncreasesNesting и
// gr12.DecreasesNesting. Ошибка означает дефект конфигурации; неизвестную версию
// входного файла диагностирует сам parser.
func prepareGrammar() (grammars grammar.Registry, assertions map[string]map[grammar.AssertionID]grammar.AssertionFunc, err error) {
	grammars = grammar.NewRegistry()
	checks := gr12.BuiltinGr12(grammars)
	registered, found := grammars.Lookup(supportedDSLVersion)
	if !found {
		return nil, nil, fmt.Errorf("грамматика %q не зарегистрирована", supportedDSLVersion)
	}
	enter, leave := registered.Assertions()
	if enter != gr12.IncreasesNesting || leave != gr12.DecreasesNesting {
		return nil, nil, fmt.Errorf("грамматика %q: некорректные структурные роли %q и %q", supportedDSLVersion, enter, leave)
	}
	for _, id := range []grammar.AssertionID{enter, leave} {
		if checks[id] == nil {
			return nil, nil, fmt.Errorf("грамматика %q: не задана проверка %q", supportedDSLVersion, id)
		}
	}
	assertions = map[string]map[grammar.AssertionID]grammar.AssertionFunc{
		supportedDSLVersion: {
			enter: checks[enter],
			leave: checks[leave],
		},
	}
	return grammars, assertions, nil
}
