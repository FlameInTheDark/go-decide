package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/FlameInTheDark/go-decide/internal/cliconfig"
	"github.com/FlameInTheDark/go-decide/providers/ollama"
)

// The two decisions this command makes are worth pinning: it refuses to listen
// anywhere but loopback, and it turns a non-loopback address into an error
// before anything is bound.
func TestCheckLoopbackAcceptsOnlyLoopbackAddresses(t *testing.T) {
	tests := []struct {
		addr    string
		wantErr bool
	}{
		{"127.0.0.1:842", false},
		{"localhost:842", false},
		{"[::1]:842", false},
		{"0.0.0.0:842", true},
		{"192.168.1.10:842", true},
		{"example.com:842", true},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			err := checkLoopback(tt.addr)
			if tt.wantErr && err == nil {
				t.Error("expected an error: this server has no authentication")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestCheckLoopbackMessageExplainsWhy matters because the only reason for the
// restriction is the missing authentication: a user who hits it deserves to
// know that, not just "invalid address".
func TestCheckLoopbackMessageExplainsWhy(t *testing.T) {
	err := checkLoopback("0.0.0.0:842")
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); got == "" {
		t.Error("the error explains nothing")
	}
}

// TestDisplayURLPrefersLocalhost keeps the printed URL clickable: a browser
// resolves localhost to the loopback the server actually bound.
func TestDisplayURLPrefersLocalhost(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{"127.0.0.1:842", "http://localhost:842"},
		{"localhost:842", "http://localhost:842"},
		{"[::1]:842", "http://localhost:842"},
	}
	for _, tt := range tests {
		if got := displayURL(tt.addr); got != tt.want {
			t.Errorf("displayURL(%q) = %q, want %q", tt.addr, got, tt.want)
		}
	}
}

// TestEffectiveBaseURLFillsTheProviderDefault means an unset flag does not
// reach the provider as an empty address.
func TestEffectiveBaseURLFillsTheProviderDefault(t *testing.T) {
	if got := effectiveBaseURL(cliconfig.Settings{Provider: ollama.Name}); got == "" {
		t.Error("expected the provider's default address")
	}
	want := "http://box:11434"
	if got := effectiveBaseURL(cliconfig.Settings{Provider: ollama.Name, BaseURL: want}); got != want {
		t.Errorf("effectiveBaseURL = %q, want the explicit address", got)
	}
}

// TestVersionIsOverridable guards the release pipeline's -X main.version flag:
// without this var the build flag would silently do nothing.
func TestVersionIsOverridable(t *testing.T) {
	if version == "" {
		t.Error("version is empty")
	}
}

// TestListenErrorStripsFibersRepeatedPrefix: Fiber prefixes "failed to listen: "
// twice, once in Listen and again in its listener helper, so the cause that
// actually says what to do arrives buried under the same words twice.
func TestListenErrorStripsFibersRepeatedPrefix(t *testing.T) {
	inner := errors.New("listen tcp4 127.0.0.1:842: bind: address already in use")
	doubled := fmt.Errorf("failed to listen: failed to listen: %w", inner)

	got := listenError(doubled).Error()
	if strings.HasPrefix(got, "failed to listen") {
		t.Errorf("listenError kept Fiber's prefix: %q", got)
	}
	if !strings.Contains(got, "address already in use") {
		t.Errorf("listenError lost the cause: %q", got)
	}
	if strings.Contains(got, "failed to listen: failed to listen") {
		t.Errorf("the message is still doubled: %q", got)
	}
}

func TestListenErrorLeavesAnUnrelatedMessageAlone(t *testing.T) {
	original := errors.New("the key is invalid")
	if got := listenError(original); got.Error() != original.Error() {
		t.Errorf("listenError rewrote %q into %q", original, got)
	}
}

// TestVersionDoesNotServe: asking for a version must not bind a port. The CLI
// had the same shape and shipped with no version subcommand at all, so the
// subcommand is what gets run first, by habit, from a script.
func TestVersionDoesNotServe(t *testing.T) {
	for _, args := range [][]string{
		{"version"},
		{"--version"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			command := &cli.Command{
				Name:     "decide-playground",
				Version:  version,
				Writer:   &strings.Builder{},
				Action:   run,
				Commands: []*cli.Command{versionCommand()},
				Flags:    newCommand().Flags,
			}

			done := make(chan error, 1)
			argv := append([]string{"decide-playground"}, args...)
			go func() { done <- command.Run(context.Background(), argv) }()

			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("Run: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("started a server instead of printing a version")
			}
		})
	}
}
