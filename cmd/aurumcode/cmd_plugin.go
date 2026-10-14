// `aurumcode plugin`: the external plugins of the scheduled run.
// list prints the trusted configuration's plugins; ingest turns a plugin's
// output into a SARIF document for code scanning (internal/plugin), and
// destino decides where findings may go from the repository's visibility.
// The rules live in internal/plugin; this file only parses flags.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/plugin"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

type pluginFlags struct {
	repo, politica, name, format, input, sarif, idioma *string
	harnessSARIF, harnessErrors, visibilidade, destino *string
	pluginExit                                         *int
	asJSON, codeScanning                               *bool
}

func newPluginFlagSet() (*flag.FlagSet, pluginFlags) {
	fs := flag.NewFlagSet("plugin", flag.ContinueOnError)
	return fs, pluginFlags{
		repo:          fs.String("repo", ".", "list: checkout of the default branch; ingest: the scanned tree the findings' paths are relative to"),
		politica:      fs.String("politica", "", "list: central policy directory (AURUMCODE_POLICY when empty)"),
		asJSON:        fs.Bool("json", false, "list: print the plugins as JSON"),
		name:          fs.String("name", "", "ingest: the plugin's name (scheduled.plugins[].name)"),
		format:        fs.String("format", "", "ingest: sarif or vvah-findings"),
		input:         fs.String("input", "", "ingest: the file the plugin wrote"),
		sarif:         fs.String("sarif", "", "ingest: SARIF output path; not written when the run is inconclusive"),
		pluginExit:    fs.Int("plugin-exit", -1, "ingest: the plugin's exit code (required; != 0 is inconclusive)"),
		harnessSARIF:  fs.String("harness-sarif", "", "ingest vvah-findings: the harness's own SARIF (default: newest *_report.sarif next to --input)"),
		harnessErrors: fs.String("harness-errors", "", "ingest vvah-findings: the harness's errors.jsonl (default: newest *_errors.jsonl next to --input)"),
		idioma:        fs.String("idioma", string(i18n.Portuguese), "ingest: language of the counts line (pt-BR or en)"),
		visibilidade:  fs.String("visibilidade", "", "destino: the repository's visibility (public, private or internal)"),
		codeScanning:  fs.Bool("code-scanning", false, "destino: code scanning is available in the repository"),
		destino:       fs.String("destino", "", "destino: check a requested destination (security or issues) instead of choosing one"),
	}
}

func pluginSubcommand() subcommand {
	return subcommand{
		name:       "plugin",
		docSection: "Plugins externos da validação agendada (`aurumcode plugin`)",
		summary:    "List the scheduled run's external plugins (list), ingest a plugin's output into SARIF (ingest) or choose where findings go (destino).",
		example:    "aurumcode plugin ingest --name vvah --format vvah-findings --input security-scan/findings.json --repo . --plugin-exit 0 --sarif plugin-vvah.sarif",
		flags:      func() *flag.FlagSet { fs, _ := newPluginFlagSet(); return fs },
		run:        runPlugin,
	}
}

func runPlugin(args []string, stdout, stderr io.Writer, filter *redaction.Filter) int {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		if exit, ok := parseSubcommandFlags("plugin", flag.NewFlagSet("plugin", flag.ContinueOnError), args, stdout, stderr); !ok {
			return exit
		}
		fmt.Fprintln(stderr, "aurumcode plugin: missing verb (list, ingest or destino)")
		return 2
	}
	verb := args[0]
	fs, fl := newPluginFlagSet()
	if exit, ok := parseSubcommandFlags("plugin", fs, args[1:], stdout, stderr); !ok {
		return exit
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "aurumcode plugin: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	switch verb {
	case "list":
		return runPluginList(fl, stdout, stderr)
	case "ingest":
		return runPluginIngest(fl, stdout, stderr, filter)
	case "destino":
		return runPluginDestination(fl, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "aurumcode plugin: unknown verb %q (list, ingest or destino)\n", verb)
		return 2
	}
}

func runPluginList(fl pluginFlags, stdout, stderr io.Writer) int {
	policyDir := strings.TrimSpace(*fl.politica)
	if policyDir == "" {
		policyDir = strings.TrimSpace(os.Getenv("AURUMCODE_POLICY"))
	}
	cfg, _, warnings, err := loadEffectiveConfig(strings.TrimSpace(*fl.repo), policyDir)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode plugin list: %v\n", err)
		return 2
	}
	for _, w := range warnings {
		fmt.Fprintf(stderr, "aurumcode plugin list: %s: %s\n", w.Provider, w.Reason)
	}
	plugins := pluginListEntries(cfg.Scheduled.EffectivePlugins())
	if *fl.asJSON {
		data, err := json.Marshal(plugins)
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode plugin list: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(data))
		return 0
	}
	for _, p := range plugins {
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", p.Name, p.Output.Format, p.Output.Path)
	}
	return 0
}

// pluginListEntry is one plugin as the workflow reads it.
type pluginListEntry struct {
	Name           string   `json:"name"`
	Image          string   `json:"image,omitempty"`
	Dockerfile     string   `json:"dockerfile,omitempty"`
	Args           []string `json:"args"`
	Env            []string `json:"env"`
	TimeoutMinutes int      `json:"timeout_minutes"`
	JobTimeout     int      `json:"job_timeout_minutes"`
	Output         struct {
		Format string `json:"format"`
		Path   string `json:"path"`
	} `json:"output"`
}

func pluginListEntries(plugins []config.PluginConfig) []pluginListEntry {
	out := make([]pluginListEntry, 0, len(plugins))
	for _, p := range plugins {
		e := pluginListEntry{Name: p.Name, Image: p.Image, Dockerfile: p.Dockerfile, Args: p.Args, Env: p.Env, TimeoutMinutes: p.EffectiveTimeoutMinutes(), JobTimeout: p.JobTimeoutMinutes()}
		if e.Args == nil {
			e.Args = []string{}
		}
		if e.Env == nil {
			e.Env = []string{}
		}
		e.Output.Format, e.Output.Path = p.Output.Format, p.Output.Path
		out = append(out, e)
	}
	return out
}

func runPluginIngest(fl pluginFlags, stdout, stderr io.Writer, filter *redaction.Filter) int {
	if *fl.pluginExit < 0 {
		fmt.Fprintln(stderr, "aurumcode plugin ingest: --plugin-exit is required (the plugin's exit code)")
		return 2
	}
	return plugin.Run(plugin.Options{
		Name:          strings.TrimSpace(*fl.name),
		Format:        strings.TrimSpace(*fl.format),
		Input:         strings.TrimSpace(*fl.input),
		Repo:          strings.TrimSpace(*fl.repo),
		SARIF:         strings.TrimSpace(*fl.sarif),
		PluginExit:    *fl.pluginExit,
		ToolVersion:   toolVersion().Label(),
		HarnessSARIF:  strings.TrimSpace(*fl.harnessSARIF),
		HarnessErrors: strings.TrimSpace(*fl.harnessErrors),
		Filter:        filter,
	}, *fl.idioma, stdout, stderr)
}

func runPluginDestination(fl pluginFlags, stdout, stderr io.Writer) int {
	if d := strings.TrimSpace(*fl.destino); d != "" {
		if err := plugin.CheckDestination(*fl.visibilidade, d); err != nil {
			fmt.Fprintf(stderr, "aurumcode plugin destino: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, d)
		return 0
	}
	fmt.Fprintln(stdout, plugin.Destination(*fl.visibilidade, *fl.codeScanning))
	return 0
}
