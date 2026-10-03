package ollama

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelsAndVersion(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case TagsPath:
			_, _ = io.WriteString(w, `{"models":[
              {"name":"nimble:latest","model":"nimble:latest","size":100},
              {"name":"llama3:latest","model":"llama3:latest","size":200},
              {"name":"clef-flash:latest","model":"clef-flash:latest","capabilities":["vision"]}
            ]}`)
		case VersionPath:
			_, _ = io.WriteString(w, `{"version":"0.35.1"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	provider := New(WithBaseURL(server.URL))

	models, err := provider.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("models = %d, want 3", len(models))
	}

	decision := FilterDecisionModels(models)
	if len(decision) != 2 {
		t.Fatalf("decision models = %d, want 2", len(decision))
	}
	if !decision[1].Vision() {
		t.Error("clef-flash should report vision")
	}

	names, err := provider.DecisionModelNames(context.Background())
	if err != nil {
		t.Fatalf("DecisionModelNames: %v", err)
	}
	if len(names) != 2 || names[0] != "nimble:latest" {
		t.Errorf("names = %v", names)
	}

	supported, err := provider.SupportsSystemOne(context.Background())
	if err != nil {
		t.Fatalf("SupportsSystemOne: %v", err)
	}
	if !supported {
		t.Error("0.35.1 should support System One")
	}

	for _, call := range paths {
		if call != "GET "+TagsPath && call != "GET "+VersionPath {
			t.Errorf("unexpected call %q", call)
		}
	}
}

func TestSupportsSystemOneTooOld(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"version":"0.34.0"}`)
	}))
	t.Cleanup(server.Close)

	supported, err := New(WithBaseURL(server.URL)).SupportsSystemOne(context.Background())
	if err != nil {
		t.Fatalf("SupportsSystemOne: %v", err)
	}
	if supported {
		t.Error("0.34.0 should not support System One")
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		raw       string
		wantMajor int
		wantMinor int
		wantPatch int
		wantErr   bool
	}{
		{"0.35.0", 0, 35, 0, false},
		{"v0.35.1", 0, 35, 1, false},
		{"1.2.3", 1, 2, 3, false},
		{"0.35", 0, 35, 0, false},
		{"nope", 0, 0, 0, true},
	}

	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := parseVersion(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseVersion: %v", err)
			}
			if got.major != tc.wantMajor || got.minor != tc.wantMinor || got.patch != tc.wantPatch {
				t.Errorf("got %v, want %d.%d.%d", got, tc.wantMajor, tc.wantMinor, tc.wantPatch)
			}
		})
	}
}
