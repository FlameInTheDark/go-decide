// Package all registers every decision provider bundled with go-decide.
//
// Import it for its side effects when you want decide.New() to resolve any
// bundled adapter by name:
//
//	import _ "github.com/FlameInTheDark/go-decide/providers/all"
package all

import (
	_ "github.com/FlameInTheDark/go-decide/providers/ollama"
	_ "github.com/FlameInTheDark/go-decide/providers/openrouter"
)
