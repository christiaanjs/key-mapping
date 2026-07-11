// Command tui is the terminal frontend for the keymap trainer. It wraps the
// pure core.App (see core/app.go) in a Bubble Tea program: Update maps
// keypresses to core.Event values and calls Dispatch; View renders the
// returned core.State with Lip Gloss.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/christiaanswanepoel/key-mapping/core"
	"github.com/christiaanswanepoel/key-mapping/corpus"
	"github.com/christiaanswanepoel/key-mapping/mappings"
)

func main() {
	corpusKind := flag.String("corpus", "static", `corpus source: "static", "file", "code", or "ollama"`)
	corpusPath := flag.String("corpus-path", "", "path for -corpus=file (a text file) or -corpus=code (a directory root)")
	ollamaModel := flag.String("ollama-model", "", "Ollama model to use with -corpus=ollama (empty uses the corpus package default)")
	ollamaHost := flag.String("ollama-host", "", "Ollama host URL to use with -corpus=ollama (empty uses OLLAMA_HOST or the client default)")
	flag.Parse()

	// A streaming corpus keeps a producer goroutine alive for the life of the
	// program, so its context is cancelled on exit rather than timed out — the
	// drill is meant to keep pulling fresh text for as long as it runs.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mapping := buildMapping()
	corp := buildCorpus(ctx, *corpusKind, *corpusPath, *ollamaModel, *ollamaHost)

	app := core.New(mapping, corp)

	p := tea.NewProgram(newModel(app), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tui: ", err)
		os.Exit(1)
	}
}

// buildMapping parses the embedded default mapping set, falling back to the
// built-in static mapping on error.
func buildMapping() core.Mapping {
	m, err := core.ParseMapping(mappings.FS, mappings.Default)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tui: warning: failed to parse embedded mapping %q: %v; falling back to static mapping\n", mappings.Default, err)
		return core.NewStaticMapping()
	}
	return m
}

// buildCorpus selects a corpus based on the -corpus flag, printing a warning to
// stderr and falling back to the static corpus on any failure. All output
// happens here, before the Bubble Tea program takes over the screen.
//
// The file and code sources load their whole bank up front (a local read, so it
// is fast). The ollama source does NOT: it returns immediately and streams text
// in from the model on ctx's goroutine, serving static text until the first
// items land. The TUI reports that live via State.Corpus, so a slow model shows
// as "warming" rather than a frozen startup.
func buildCorpus(ctx context.Context, kind, path, model, host string) core.Corpus {
	switch kind {
	case "static":
		fmt.Fprintln(os.Stderr, "tui: using corpus: static")
		return core.NewStaticCorpus()

	case "file":
		if path == "" {
			fmt.Fprintln(os.Stderr, "tui: warning: -corpus=file requires -corpus-path; falling back to static corpus")
			return core.NewStaticCorpus()
		}
		src, err := corpus.FromFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tui: warning: failed to load corpus file %q: %v; falling back to static corpus\n", path, err)
			return core.NewStaticCorpus()
		}
		fmt.Fprintf(os.Stderr, "tui: using corpus: file (%s)\n", path)
		return src

	case "code":
		if path == "" {
			fmt.Fprintln(os.Stderr, "tui: warning: -corpus=code requires -corpus-path; falling back to static corpus")
			return core.NewStaticCorpus()
		}
		src, err := corpus.FromCodebase(path, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "tui: warning: failed to load corpus from codebase %q: %v; falling back to static corpus\n", path, err)
			return core.NewStaticCorpus()
		}
		fmt.Fprintf(os.Stderr, "tui: using corpus: code (%s)\n", path)
		return src

	case "ollama":
		// Only a malformed host fails here; an unreachable or slow server is a
		// soft failure the stream reports through State.Corpus while the drill
		// carries on against the static fallback.
		src, err := corpus.FromOllama(ctx, corpus.Options{Model: model, Host: host}, core.NewStaticCorpus())
		if err != nil {
			fmt.Fprintf(os.Stderr, "tui: warning: failed to build ollama corpus: %v; falling back to static corpus\n", err)
			return core.NewStaticCorpus()
		}
		fmt.Fprintln(os.Stderr, "tui: using corpus: ollama (streaming in the background)")
		return src

	default:
		fmt.Fprintf(os.Stderr, "tui: warning: unknown -corpus value %q; falling back to static corpus\n", kind)
		return core.NewStaticCorpus()
	}
}
