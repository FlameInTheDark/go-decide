// Command decide-playground serves a local web UI for exercising the decide
// library.
//
// It is a single binary: the React frontend is embedded with go:embed, and
// every decision it runs goes through decide.Client, so validation, capability
// checks, retries and error classification behave exactly as they do in the
// decide command.
//
// The server binds to loopback and has no authentication. Configuration lives
// only in memory, so nothing is written to disk and nothing survives a restart.
//
// Usage:
//
//	decide-playground [flags]
//
// Flags:
//
//	--provider value    provider to use: ollama, openrouter (default "ollama")
//	--base-url value    provider base URL
//	--api-key value     provider API key
//	--model value       default model
//	--timeout value     per-decision timeout (default 2m0s)
//	--retries value     retries after a failed attempt (default 2)
//	--listen value      address to bind (default "127.0.0.1:842")
//	--open              open the playground in a browser
//	--verbose           log every request
//	--help              show help
//	--version           show version
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/FlameInTheDark/go-decide/internal/cliconfig"
	"github.com/FlameInTheDark/go-decide/internal/playground"
	"github.com/FlameInTheDark/go-decide/providers/ollama"
	"github.com/FlameInTheDark/go-decide/providers/openrouter"
	"github.com/FlameInTheDark/go-decide/web"
)

// version is overridden at build time with -X main.version=...
var version = "dev"

func main() {
	if err := newCommand().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "decide-playground:", err)
		os.Exit(1)
	}
}

func newCommand() *cli.Command {
	return &cli.Command{
		Name:    "decide-playground",
		Usage:   "serve a local web playground for the decide library",
		Version: version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "provider",
				Aliases: []string{"p"},
				Usage:   "provider to use: ollama, openrouter",
				Value:   ollama.Name,
			},
			&cli.StringFlag{
				Name:  "base-url",
				Usage: "provider base URL, defaults to the provider's own",
			},
			&cli.StringFlag{
				Name:    "api-key",
				Aliases: []string{"k"},
				Usage:   "API key for providers that need one, kept in memory only",
			},
			&cli.StringFlag{
				Name:    "model",
				Aliases: []string{"m"},
				Usage:   "default model for decisions",
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "per-decision `timeout`",
				Value: cliconfig.DefaultTimeout,
			},
			&cli.IntFlag{
				Name:  "retries",
				Usage: "retries after a failed attempt",
				Value: cliconfig.DefaultRetries,
			},
			&cli.StringFlag{
				Name:  "listen",
				Usage: "address to bind, loopback by default",
				Value: "127.0.0.1:842",
			},
			&cli.BoolFlag{
				Name:  "open",
				Usage: "open the playground in a browser once it is serving",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "log every request to stderr",
			},
		},
		Action: run,
		Commands: []*cli.Command{
			versionCommand(),
		},
	}
}

// listenError unwraps the repetition Fiber puts in a listen failure: both its
// Listen and its listener helper prefix the same "failed to listen: " message,
// so the underlying error arrives as
//
//	failed to listen: failed to listen: listen tcp4 127.0.0.1:842: bind: ...
//
// The cause underneath is the part that says what to do about it.
func listenError(err error) error {
	const prefix = "failed to listen: "
	message := err.Error()
	for strings.HasPrefix(message, prefix) {
		message = strings.TrimPrefix(message, prefix)
	}
	if message == err.Error() {
		return err
	}
	return errors.New(message)
}

func versionCommand() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Print the decide-playground version",
		Action: func(_ context.Context, cmd *cli.Command) error {
			fmt.Fprintf(cmd.Root().Writer, "decide-playground %s\n", version)
			return nil
		},
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	listen := cmd.String("listen")
	if err := checkLoopback(listen); err != nil {
		return err
	}

	assets, err := web.Assets()
	if err != nil {
		return err
	}

	settings := cliconfig.Settings{
		Provider: cmd.String("provider"),
		BaseURL:  cmd.String("base-url"),
		APIKey:   cmd.String("api-key"),
		Model:    cmd.String("model"),
		Timeout:  cmd.Duration("timeout"),
		Retries:  cmd.Int("retries"),
		Verbose:  cmd.Bool("verbose"),
	}.WithDefaults()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	settings.Logger = logger

	server, err := playground.New(playground.Options{
		Settings: settings,
		Logger:   logger,
		Assets:   assets,
	})
	if err != nil {
		return err
	}

	url := displayURL(listen)
	fmt.Fprintf(os.Stderr, "decide-playground %s on %s\n", version, url)
	fmt.Fprintf(os.Stderr, "provider %s at %s\n", settings.Provider, effectiveBaseURL(settings))
	if web.IsPlaceholder() {
		fmt.Fprintln(os.Stderr, "warning: this binary embeds the placeholder frontend; run `make web` and rebuild for the full UI")
	}
	fmt.Fprintln(os.Stderr, "press ctrl-c to stop")

	if cmd.Bool("open") {
		go openBrowser(url)
	}

	// Shut down cleanly so an in-flight decision is given a chance to finish.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	listenErr := make(chan error, 1)
	go func() { listenErr <- server.Listen(listen) }()

	select {
	case err := <-listenErr:
		if err != nil {
			return listenError(err)
		}
		return nil
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

// checkLoopback refuses to expose the server beyond the local machine. The UI
// has no authentication, so binding to a routable address would let anyone on
// the network spend the user's API key.
func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("--listen %q must be host:port: %w", addr, err)
	}
	if host == "" {
		return fmt.Errorf("--listen %q has no host, pass something like 127.0.0.1:842", addr)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("--listen host %q must be an IP address or localhost, this server has no authentication", host)
	}
	if !ip.IsLoopback() {
		return fmt.Errorf("--listen host %q is not a loopback address, this server has no authentication and may only bind localhost", host)
	}
	return nil
}

// displayURL turns a listen address into a URL a browser can open, using
// localhost rather than 127.0.0.1 so the origin matches what a user types.
func displayURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func effectiveBaseURL(settings cliconfig.Settings) string {
	if settings.BaseURL != "" {
		return settings.BaseURL
	}
	switch settings.Provider {
	case ollama.Name:
		return ollama.DefaultBaseURL
	case openrouter.Name:
		return openrouter.DefaultBaseURL
	default:
		return "(provider default)"
	}
}

// openBrowser asks the desktop for the default browser. Failure is not worth
// reporting: the URL is already on stderr.
func openBrowser(url string) {
	time.Sleep(300 * time.Millisecond)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Run()
}
