package runner

import (
	"context"

	"leil.io/sfstests/internal/utils"
)

type Runner interface {
	Setup(options utils.TestOptions, ctx context.Context)
	RunTest(suite string, name string) (succeded bool, output string)
	// 'name' must handle wildcard (*) pattern
	GetTests(name string) (tests map[string][]string)
}
