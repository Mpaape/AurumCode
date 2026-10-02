// Command analysis-data builds, verifies and names the analysis-data
// artifact. It is run by scripts/artifacts/build.sh inside the scheduled
// workflow; it is not part of the product binary.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/Mpaape/AurumCode/internal/artifacts"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: analysis-data build|verify|tag|digest [flags]")
		return 64
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	dir := fs.String("dir", "dist", "artifact directory")
	osv := fs.String("osv-base", artifacts.DefaultOSVBase, "public OSV source base URL")
	scanners := fs.String("scanners", "", "scanner pin file (scanners.yml)")
	if err := fs.Parse(args[1:]); err != nil {
		return 64
	}
	switch args[0] {
	case "build":
		m, err := artifacts.Build(context.Background(), artifacts.BuildOptions{OSVBase: *osv, ScannersFile: *scanners, OutDir: *dir})
		if err != nil {
			fmt.Fprintln(os.Stderr, "analysis-data build:", err)
			return 1
		}
		fmt.Printf("built %d files, set_digest=%s generated_at=%s\n", len(m.Files), m.SetDigest, m.GeneratedAt)
	case "verify":
		m, err := artifacts.VerifyDir(*dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "analysis-data verify:", err)
			return 1
		}
		fmt.Printf("verified %d files, set_digest=%s\n", len(m.Files), m.SetDigest)
	case "tag":
		m, err := artifacts.VerifyDir(*dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "analysis-data tag:", err)
			return 1
		}
		t, err := m.Tag()
		if err != nil {
			fmt.Fprintln(os.Stderr, "analysis-data tag:", err)
			return 1
		}
		fmt.Println(t)
	case "digest":
		m, err := artifacts.VerifyDir(*dir)
		if err != nil {
			fmt.Fprintln(os.Stderr, "analysis-data digest:", err)
			return 1
		}
		fmt.Println(m.SetDigest)
	default:
		fmt.Fprintln(os.Stderr, "unknown subcommand", args[0])
		return 64
	}
	return 0
}
