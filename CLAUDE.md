# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this project is

A single-user, local toolkit for **managing and practicing Karabiner-Elements key mappings**. The centerpiece is the **trainer**, an app that drills the user on a keymap. A secondary goal (later) is to **simulate** mappings and measure their typing efficiency.

The current focus is a **left-hand-only layout** (Half-QWERTY: hold spacebar to mirror the right-hand keys onto the left hand, plus a `b`-held nav layer and caps/tab remaps). The design must generalize to other layouts later, so avoid baking left-hand assumptions into shared code.

## Current state — read this first

Phases 0–5 of `PLAN.md` are built: the pure `core` (now including the mapping parser), the terminal (TUI) frontend, the web (WASM) frontend, and the pluggable `corpus` package all work and are covered by CI. What each thing is:

- `core/` — the pure trainer: `App` with `Dispatch(Event) State` / `Snapshot() State`, the four drill modes, the `Mapping` / `Corpus` **seam interfaces**, and `ParseMapping`.
- `corpus/` — pluggable corpus sources (static, file, codebase, Ollama). All corpus I/O (os, net/http) lives here, **outside** the pure core.
- `cmd/tui/` — Bubble Tea + Lip Gloss frontend. `cmd/web/` + `web/` — WASM entrypoint plus a thin vanilla-JS renderer (no framework/CDN/build step).
- `trainer/architecture.md` — the architecture this follows. `trainer/index.html` — the original **throwaway prototype**; it is the behavior/visual reference only, do not extend it.
- `mappings/qwerty-mirror/*.json` — the **real Karabiner mapping**; see the format section below. `mappings/mappings.go` embeds it as an `embed.FS` (wasm-safe) with `mappings.Default` = `"qwerty-mirror"`.

**The mapping is now parsed; the corpus is pluggable — both still behind the same interfaces.** `core.ParseMapping(fsys fs.FS, dir string) (Mapping, error)` (`core/mapping_parse.go`) reads `mappings/*.json` and builds a `mirrorTable` (`core/mirror_table.go`) — the same type `NewStaticMapping()` (`core/mapping_static.go`) builds, so parsed and static behave identically by construction. `core/corpus_static.go` (`staticCorpus`) and `corpus.Source` (`corpus/*.go`) both implement `Corpus`. `New(m Mapping, c Corpus)` injects them; `NewDefault()` wires the static ones. Both frontends parse the embedded mapping and (TUI) select a corpus, each falling back to the static implementation with an stderr warning on error. The drill and both frontends still depend only on the interfaces.

Remaining goals: Phase 6 (simulation/efficiency) and Phase 7 (LLM content generation, provider-flexible) — see `PLAN.md`.

Key seams to preserve when extending: keep `core` pure (the parser takes an `fs.FS`, never `os`; corpus I/O stays in `corpus/`). The web frontend uses the parsed mapping + static corpus only — a browser sandbox can reach neither the filesystem nor a local Ollama server.

## Target architecture (from `trainer/architecture.md`)

One **pure Go core**, two thin frontends — the core owns all state and logic and knows nothing about rendering.

```
core/        pure Go — state, logic, Event/State types. Imports NOTHING platform-specific
             (no os, no syscall/js, no Bubble Tea) so it compiles for both native and WASM.
cmd/tui/     Bubble Tea + Lip Gloss binary — imports core
cmd/web/     WASM entrypoint (GOOS=js GOARCH=wasm) — imports core, uses syscall/js
web/         static assets — HTML + thin vanilla-JS renderer + wasm_exec.js (no backend server)
```

Non-negotiable rules when building this out:

- **Event in, state out.** All mutation flows through a single `Dispatch(Event) State`; `Snapshot() State` reads current state. Both frontends speak only this vocabulary.
- **Share the model, not the view.** Each frontend renders independently (Lip Gloss for TUI, vanilla JS for web). Do not try to share rendering. (The architecture doc suggested Preact; the web layer is intentionally dependency-free vanilla JS instead — thinner, offline, no build step.)
- **JS boundary = JSON.** The web build serializes `Snapshot()` to JSON once per update and hands the blob to JS. `cmd/web` exposes `snapshot()` and `dispatch(eventJSON)` on the JS global. Treat WASM as a local API returning JSON, mirroring the TUI's `Snapshot()`.
- **No server, no locking.** Web state lives in WASM memory (persist to `localStorage`/`IndexedDB` if needed). One core instance per frontend, single-threaded event loop — no mutex on `App` unless that assumption changes.
- **Verify TinyGo early** if web binary size matters — the standard Go WASM runtime is ~2 MB+ gzipped, and TinyGo lacks full stdlib/reflection (watch `encoding/json`).

## Build & run

Use the Makefile targets:

```
make build-tui   # -> bin/app          (then: ./bin/app)
make build-web   # -> web/app.wasm + copies wasm_exec.js
make test        # go test ./...
make vet         # go vet ./...
```

Run the terminal app with `./bin/app` (or `go run ./cmd/tui`); modes switch on **F1–F4**, `ctrl+c` quits. Serve the web app from `web/` over HTTP (`cd web && python3 -m http.server 8000`) — `file://` will not work because `instantiateStreaming` needs an HTTP response.

Two build gotchas worth knowing (both are CI-enforced):
- The wasm build **must** use `-o` (`go build -o web/app.wasm ./cmd/web`); a bare `go build ./cmd/web` emits a binary named `web` that collides with the `web/` directory.
- `cmd/web/main.go` carries a `//go:build js && wasm` constraint so native `go build ./...` / `go vet ./...` skip it (it imports `syscall/js`, which only exists under `GOOS=js`). Keep that tag on any file in `cmd/web`.

`web/app.wasm` and `web/wasm_exec.js` are generated (gitignored); regenerate with `make build-web`. CI runs gofmt + vet + native/wasm builds + tests on every push (`.github/workflows/ci.yml`).

## Karabiner mapping format (`mappings/`)

Each file is a Karabiner `manipulators` array (the kind that lives under a `complex_modifications` rule). Layers are implemented with **variables**, not real modifier keys — this is the key thing the parser must model:

- `half-qwerty.json` — holding **spacebar** sets variable `alt=1` (released → `alt=0`); tapping it alone types a space (`to_if_alone`). While `alt=1`, left-hand keys map to their right-hand mirror (`q→p`, `a→;`, `f→j`, digits `1→0`…). Mappings are declared in both directions so the physical layout stays a real keyboard when `alt=0`.
- `nav.json` — holding **b** (while `alt` is *not* set) sets `nav=1`; while `nav=1`, `asdf` → arrow keys `←↓↑→`. Some `alt`-layer keys carry a `variable_unless nav` condition so nav wins over the mirror.
- `return-delete.json` — `caps_lock`→return (when `alt=0`); `tab`→delete (when `alt=1`).

A single logical mapping is spread across multiple JSON files. The parser needs to read a directory, understand `set_variable` / `variable_if` / `variable_unless` conditions and `to_if_alone` / `to_after_key_up`, and reduce them into "what keystroke produces what output under which layer state" — which is exactly the lookup the prototype's `MIRROR` / `routeFor` / `diagnose` functions fake by hand.

## Trainer behavior (from the prototype)

The prototype has four modes worth preserving as the Go trainer is built: **Mirror typing** (word bank + full sentences, live WPM/accuracy, per-keystroke hint and mistake diagnosis), **Nav layer** drill (random arrow sequences), **Scratchpad** (free typing against the live keymap), and **Reference** (auto-generated key→output table). In the rebuild these should be driven by the parsed mapping, and corpus/word content should come from the corpus subsystem below rather than an inline bank.

## Roadmap (intended, not yet built)

Keep these directions in mind so early abstractions don't preclude them:

- **Simulation & efficiency** — replay a corpus through a parsed mapping to measure cost (keystrokes, hand/finger travel, layer switches) and compare layouts.
- **Corpus extraction** — pull practice/simulation text from varied sources, e.g. a codebase, not just a static word list. Corpus is a pluggable input to both the trainer and the simulator.
- **LLM content generation** — generate trainer content via a **provider-flexible API** (e.g. Anthropic or a local model). Keep the generation interface behind an abstraction so providers are swappable. When working with the Anthropic API specifically, consult the `claude-api` skill for current model IDs and usage.
