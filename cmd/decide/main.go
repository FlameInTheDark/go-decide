// Command decide classifies text with System One decision models using any
// provider registered with the library.
//
//	decide "Our checkout has returned 500 errors since 9am."   # classify
//	decide classify --help
//	decide models                                             # list local models
//	decide version
//
// Run "decide --help" for the full command list.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli/v3"
)

// version is overridable at build time with
// -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	cmd := newRootCommand()

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(cmd.ErrWriter, "error: %v\n", err)
		os.Exit(exitCodeOf(err))
	}
}

// globalFlags are available to every subcommand.
func globalFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "provider",
			Aliases: []string{"p"},
			Value:   "ollama",
			Usage:   "decision backend to use: ollama or openrouter",
		},
		&cli.StringFlag{
			Name:  "base-url",
			Usage: "override the provider base URL",
		},
		&cli.StringFlag{
			Name:    "api-key",
			Aliases: []string{"k"},
			Usage:   "API key for hosted providers (default $OPENROUTER_API_KEY)",
		},
		&cli.DurationFlag{
			Name:    "timeout",
			Aliases: []string{"t"},
			Value:   2 * time.Minute,
			Usage:   "overall deadline for the request",
		},
		&cli.IntFlag{
			Name:    "retries",
			Aliases: []string{"r"},
			Value:   2,
			Usage:   "how many attempts to make for transient failures",
		},
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "log provider requests to stderr",
		},
	}
}

// newRootCommand assembles the command tree. It is separate from main so tests
// can drive the CLI without touching os.Args or os.Exit.
//
// Note: ReadArgsFromStdin is deliberately NOT enabled. urfave/cli applies it to
// the root command before dispatching, so it would block on a ReadRune loop for
// every invocation, including "version" and "--help", whenever stdin is a pipe
// or a terminal that never reaches EOF. Stdin support lives in
// settings.loadState instead, and only reads when the input is genuinely piped.
func newRootCommand() *cli.Command {
	return &cli.Command{
		Name:                  "decide",
		Usage:                 "Classify text with System One decision models",
		Version:               version,
		Description:           "Ask typed questions about a piece of text and get calibrated probabilities back, from a local Ollama server or a hosted provider such as OpenRouter.",
		Flags:                 globalFlags(),
		DefaultCommand:        "classify",
		EnableShellCompletion: true,
		Suggest:               true,
		Writer:                os.Stdout,
		ErrWriter:             os.Stderr,
		Commands: []*cli.Command{
			classifyCommand(),
			modelsCommand(),
			versionCommand(),
		},
	}
}

func versionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Print the decide version",
		Action: func(_ context.Context, cmd *cli.Command) error {
			fmt.Fprintf(cmd.Root().Writer, "decide %s\n", version)
			return nil
		},
	}
}
