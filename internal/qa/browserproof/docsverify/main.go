// Command docsverify is the executable shape of the promise
// `aurumcode docs verify --url <published-url>` (AUR-429): after publishing,
// open the home page, follow a link of the index and confirm the expected
// content is there, instead of trusting that the file was uploaded.
//
// The verification never reaches the network: it publishes the generated
// documentation tree (--docs) through the sitepublish stand-in, serves it on
// loopback and navigates it with the pinned driver. --url names the public
// location the verdict is about and is recorded in the output. Without
// AURUM_BROWSERPROOF_DRIVER pinning a real headless browser, navigation runs
// through the in-process scripted driver and the verdict says so — the
// driver identity is part of the embedded proof.
//
// Exit codes:
//
//	0  proved: the published site opens, navigates, and shows the content
//	1  refused: the site does not deliver the promise (the verdict says why)
//	69 inconclusive: the environment could not judge the site
//	64 usage: the invocation itself is wrong
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/Mpaape/AurumCode/internal/qa/browserproof"
	"github.com/Mpaape/AurumCode/internal/qa/browserproof/sitepublish"
)

const (
	exitProved       = 0
	exitRefused      = 1
	exitUsage        = 64
	exitInconclusive = 69

	defaultIndexSelector   = "h2"
	defaultIndexText       = "Generated API documentation"
	defaultContentSelector = "body"
	runTimeout             = 240 * time.Second
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("docsverify", flag.ContinueOnError)
	flags.SetOutput(stderr)

	var (
		publishedURL    = flags.String("url", "", "public URL the site is published under (required; recorded in the verdict)")
		docsDir         = flags.String("docs", "", "generated documentation directory to verify (required)")
		contentText     = flags.String("content", "", "content a page linked from the index must show (required)")
		contentSelector = flags.String("content-selector", defaultContentSelector, "selector the content must appear under")
		indexText       = flags.String("index-text", defaultIndexText, "text the home page must show")
		indexSelector   = flags.String("index-selector", defaultIndexSelector, "selector the index text must appear under")
		card            = flags.String("card", "AUR-429", "card or run this verdict is evidence for")
		deadline        = flags.Duration("deadline", 15*time.Second, "per-navigation deadline")
	)
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(stderr, "docsverify: unexpected argument %q\n", flags.Arg(0))
		return exitUsage
	}
	for name, value := range map[string]string{
		"--url": *publishedURL, "--docs": *docsDir, "--content": *contentText,
	} {
		if value == "" {
			fmt.Fprintf(stderr, "docsverify: %s is required\n", name)
			return exitUsage
		}
	}
	if info, err := os.Stat(*docsDir); err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "docsverify: --docs %q is not a readable directory\n", *docsDir)
		return exitUsage
	}

	driver, lock, driverName, err := resolveDriver()
	if err != nil {
		fmt.Fprintf(stderr, "docsverify: inconclusive: %v\n", err)
		return exitInconclusive
	}
	fmt.Fprintf(stderr, "docsverify: navigating with %s\n", driverName)

	published, err := os.MkdirTemp("", "docsverify-published-")
	if err != nil {
		fmt.Fprintf(stderr, "docsverify: inconclusive: %v\n", err)
		return exitInconclusive
	}
	defer func() { _ = os.RemoveAll(published) }()

	if _, err := sitepublish.PublishDocs(*docsDir, published); err != nil {
		fmt.Fprintf(stderr, "docsverify: inconclusive: %v\n", err)
		return exitInconclusive
	}

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	result, verifyErr := browserproof.New(driver).VerifyDocs(ctx, browserproof.DocsVerifyRequest{
		Card:               *card,
		SiteDir:            published,
		PublishedURL:       *publishedURL,
		IndexSelector:      *indexSelector,
		IndexText:          *indexText,
		ContentSelector:    *contentSelector,
		ContentText:        *contentText,
		DriverLock:         lock,
		NavigationDeadline: *deadline,
	})

	encoded, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(stderr, "docsverify: inconclusive: encode verdict: %v\n", err)
		return exitInconclusive
	}
	fmt.Fprintf(stdout, "%s\n", encoded)

	switch result.Outcome {
	case browserproof.OutcomeProved:
		// Trust nothing that does not survive the contract, including this
		// program's own run.
		if validateErr := result.Validate(); validateErr != nil || verifyErr != nil {
			fmt.Fprintf(stderr, "docsverify: inconclusive: proved verdict rejected: %v %v\n", validateErr, verifyErr)
			return exitInconclusive
		}
		return exitProved
	case browserproof.OutcomeRefused:
		fmt.Fprintf(stderr, "docsverify: refused: %s: %s\n", result.Code, result.Detail)
		return exitRefused
	default:
		fmt.Fprintf(stderr, "docsverify: inconclusive: %s: %s\n", result.Code, result.Detail)
		return exitInconclusive
	}
}

// resolveDriver reports honestly what will navigate: the pinned external
// headless browser when AURUM_BROWSERPROOF_DRIVER configures one, otherwise
// the in-process scripted driver, locked as itself so it can never pass for
// a browser.
func resolveDriver() (browserproof.Driver, browserproof.DriverLock, string, error) {
	if path := os.Getenv(browserproof.DriverPathEnv); path != "" {
		digest, err := browserproof.DriverDigestOf(path)
		if err != nil {
			return nil, browserproof.DriverLock{}, "", fmt.Errorf(
				"%s=%s is configured but unusable: %w", browserproof.DriverPathEnv, path, err)
		}
		return browserproof.NewExternalDriver(path),
			browserproof.DriverLock{Kind: browserproof.ExternalDriverKind, Digest: digest},
			"pinned external headless browser " + digest,
			nil
	}

	return browserproof.NewScriptedDriver(),
		browserproof.DriverLock{Kind: browserproof.ScriptedDriverKind, Digest: browserproof.ScriptedDriverDigest},
		"offline scripted driver (no real browser is pinned)",
		nil
}
