package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/urfave/cli/v3"

	"github.com/FlameInTheDark/go-decide/providers/ollama"
)

func modelsCommand() *cli.Command {
	return &cli.Command{
		Name:      "models",
		Aliases:   []string{"m"},
		Usage:     "List the decision models available on an Ollama server",
		ArgsUsage: "[name...]",
		Description: "Queries the Ollama server for installed models and reports which of them can serve decision requests.\n" +
			"With names given, it reports the capabilities of just those models.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "json",
				Aliases: []string{"j"},
				Usage:   "print the model list as JSON",
			},
			&cli.BoolFlag{
				Name:  "all",
				Usage: "include models that cannot serve decisions",
			},
		},
		Action: runModels,
	}
}

// ollamaModelView is the display-independent view of an installed model, shared
// by the table renderer and the JSON output.
type ollamaModelView struct {
	id            string
	decision      bool
	vision        bool
	cloud         bool
	size          int64
	family        string
	contextLength int
	capabilities  []string
}

// modelViews converts provider models into views.
func modelViews(models []ollama.Model) []ollamaModelView {
	views := make([]ollamaModelView, 0, len(models))
	for _, model := range models {
		views = append(views, ollamaModelView{
			id:            model.ID(),
			decision:      model.Decision(),
			vision:        model.Vision(),
			cloud:         model.Cloud(),
			size:          model.Size,
			family:        model.Family(),
			contextLength: model.ContextLength(),
			capabilities:  model.Capabilities,
		})
	}
	return views
}

func runModels(ctx context.Context, cmd *cli.Command) error {
	cfg := resolveSettings(cmd)
	provider := ollama.New(
		ollama.WithBaseURL(or(cfg.baseURL, ollama.DefaultBaseURL)),
		ollama.WithHTTPClient(httpClientFor(cfg.timeout)),
	)

	reqCtx, cancel := cfg.context(ctx)
	defer cancel()

	models, err := provider.Models(reqCtx)
	if err != nil {
		return exitError(err)
	}

	if names := positionalArgs(cmd); len(names) > 0 {
		return reportNamedModels(cfg.stdout, models, names, cmd.Bool("json"))
	}

	version, err := provider.ServerVersion(reqCtx)
	if err != nil {
		return exitError(err)
	}
	supported, err := provider.SupportsSystemOne(reqCtx)
	if err != nil {
		return exitError(err)
	}

	selected := models
	if !cmd.Bool("all") {
		selected = ollama.FilterDecisionModels(models)
	}

	server := jsonServer{
		BaseURL:           provider.BaseURL(),
		Version:           version,
		SupportsSystemOne: supported,
	}

	if cmd.Bool("json") {
		return writeJSON(cfg.stdout, buildModelsReport(server, modelViews(selected), nil))
	}

	fmt.Fprintf(cfg.stdout, "ollama %s at %s\n", version, provider.BaseURL())
	if !supported {
		fmt.Fprintln(cfg.stdout, "warning: System One needs Ollama v0.35.0 or later")
	}
	fmt.Fprintln(cfg.stdout)

	if len(selected) == 0 {
		fmt.Fprintln(cfg.stdout, "no decision models installed, run: ollama pull nimble")
		return nil
	}

	writer := tabwriter.NewWriter(cfg.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "MODEL\tSIZ\tDECISION\tVISION")
	for _, model := range modelViews(selected) {
		fmt.Fprintf(writer, "%s\t%s\t%t\t%t\n", model.id, humanSize(model.size), model.decision, model.vision)
	}
	return writer.Flush()
}

// reportNamedModels reports the capabilities of specific models, as a table or
// as JSON.
func reportNamedModels(out io.Writer, models []ollama.Model, names []string, asJSON bool) error {
	views := modelViews(models)

	if asJSON {
		found := make([]ollamaModelView, 0, len(names))
		var missing []string
		for _, name := range names {
			if view, ok := findView(views, name); ok {
				found = append(found, view)
				continue
			}
			missing = append(missing, name)
		}
		if err := writeJSON(out, buildModelsReport(jsonServer{}, found, missing)); err != nil {
			return err
		}
		if len(missing) > 0 {
			return notInstalled(missing)
		}
		return nil
	}

	writer := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "MODEL\tFOUND\tDECISION\tFAMILY\tCONTEXT")

	var missing []string
	for _, name := range names {
		view, ok := findView(views, name)
		if !ok {
			missing = append(missing, name)
			fmt.Fprintf(writer, "%s\tno\t\t\t\n", name)
			continue
		}
		fmt.Fprintf(writer, "%s\tyes\t%t\t%s\t%s\n",
			view.id, view.decision, or(view.family, "-"), or(fmt.Sprint(view.contextLength), "-"))
	}

	if err := writer.Flush(); err != nil {
		return &exitCodeError{message: err.Error(), code: exitFailure}
	}
	if len(missing) > 0 {
		return notInstalled(missing)
	}
	return nil
}

// notInstalled builds the error reported when requested models are absent.
func notInstalled(missing []string) error {
	return &exitCodeError{
		message: fmt.Sprintf("not installed: %v", missing),
		code:    exitNotFound,
	}
}

// findView looks a model up by its full name or by its name without the tag.
func findView(views []ollamaModelView, name string) (ollamaModelView, bool) {
	for _, view := range views {
		if view.id == name {
			return view, true
		}
	}
	for _, view := range views {
		if trimTag(view.id) == name {
			return view, true
		}
	}
	return ollamaModelView{}, false
}

// findModel looks a model up by its full name or by its name without the tag.
func findModel(models []ollama.Model, name string) (ollama.Model, bool) {
	for _, model := range models {
		if model.ID() == name {
			return model, true
		}
	}
	for _, model := range models {
		if trimTag(model.ID()) == name {
			return model, true
		}
	}
	return ollama.Model{}, false
}

func trimTag(name string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == ':' {
			return name[:i]
		}
	}
	return name
}

// humanSize formats a byte count in binary units.
func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
