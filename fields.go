package cmdu

import (
	"os"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// Fields performs shell expansion on s as if it were a command's arguments,
// using env to resolve variables. It is similar to Expand, but includes brace
// expansion, tilde expansion, and globbing.
//
// If env is nil, the current environment variables are used. Empty variables
// are treated as unset; to support variables which are set but empty, use the
// expand package directly.
//
// An error will be reported if the input string had invalid syntax.
func Fields(s string, env expand.Environ) ([]string, error) {
	p := syntax.NewParser()
	var words []*syntax.Word
	err := p.Words(strings.NewReader(s), func(w *syntax.Word) bool {
		words = append(words, w)
		return true
	})
	if err != nil {
		return nil, err
	}
	if env == nil {
		env = expand.ListEnviron(os.Environ()...)
	}
	cfg := &expand.Config{Env: env}
	return expand.Fields(cfg, words...)
}
