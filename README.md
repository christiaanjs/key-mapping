# key-mapping

A local, single-user toolkit for **managing and practicing [Karabiner-Elements](https://karabiner-elements.pqrs.org/) key mappings**. The centerpiece is a **trainer** that drills you on a keymap, available as both a terminal app and a browser app driven by one shared Go core.

The current focus is a **left-hand-only layout** (Half-QWERTY): hold the spacebar to mirror the right-hand keys onto the left hand, with a `b`-held nav layer and `caps`/`tab` remaps. The mappings live in [`mappings/qwerty-mirror/`](mappings/qwerty-mirror).

> **Status:** Phases 0, 2, 3, 4 of [`PLAN.md`](PLAN.md) are complete — the core, the terminal frontend, and the web frontend all work and are covered by CI. The keymap and practice corpus are currently **hardcoded behind interfaces** (`Mapping`, `Corpus`); the next step (Phase 1) is a parser that reads `mappings/` directly. See [`PLAN.md`](PLAN.md) for the roadmap.

## Architecture

One **pure Go core** owns all state and logic; two thin frontends render it. The core imports nothing platform-specific, so it compiles for both a native binary and a `GOOS=js GOARCH=wasm` web target.

```
core/        pure Go — App with Dispatch(Event) State / Snapshot() State, the four
             drill modes, and the Mapping / Corpus seam interfaces
cmd/tui/     terminal frontend — Bubble Tea + Lip Gloss
cmd/web/     WASM entrypoint — exposes snapshot()/dispatch() over syscall/js
web/         static assets — HTML + a thin vanilla-JS renderer (no framework/CDN/build)
mappings/    the real Karabiner mapping JSON (source of truth for the future parser)
trainer/     architecture.md (design) + index.html (the original prototype, reference only)
```

All mutation flows through `Dispatch(Event) State`; both frontends speak only that vocabulary and render `Snapshot()`. The web build serializes the state to JSON once per update and lets JS render the blob. See [`trainer/architecture.md`](trainer/architecture.md) for the full design and [`CLAUDE.md`](CLAUDE.md) for working notes.

### The seams

The trainer runs off hardcoded data that sits behind two interfaces, so real implementations drop in without touching the drill or the frontends:

- **`Mapping`** (`core/mapping.go`) → `staticMapping` (`core/mapping_static.go`), built from the prototype's mirror table and `mappings/*.json`. A Karabiner-JSON parser replaces it in Phase 1.
- **`Corpus`** (`core/corpus.go`) → `staticCorpus` (`core/corpus_static.go`), the prototype's word/sentence bank. Codebase extraction and LLM generation replace it in later phases.

`New(m, c)` injects both; `NewDefault()` wires the static ones.

## Build & run

Requires Go 1.26+.

```sh
# Terminal app
make build-tui        # -> bin/app
./bin/app             # F1–F4 switch modes; ctrl+c quits

# Web app
make build-web        # -> web/app.wasm (+ copies wasm_exec.js)
cd web && python3 -m http.server 8000
# open http://localhost:8000  (file:// won't work — needs HTTP)
```

Other targets: `make test`, `make vet`.

### Trainer modes

- **Mirror** — type words or sentences left-handed; the app shows the next key to press (and whether to hold space), live WPM/accuracy, and diagnoses mistakes.
- **Nav** — drill the nav-layer arrow sequences.
- **Scratch** — free typing against your live keymap.
- **Reference** — the key → output table.

Terminal keys: `F1`–`F4` switch modes; in mirror mode `ctrl+n` skips, `ctrl+w` toggles words/sentences, `ctrl+l` cycles length.

## Development

CI (`.github/workflows/ci.yml`) runs gofmt + `go vet` + native and wasm builds + `go test` on every push.

Two things to know when touching the web target:

- Build the wasm with `-o` (`go build -o web/app.wasm ./cmd/web`) — a bare `go build ./cmd/web` emits a binary named `web` that collides with the `web/` directory.
- Files in `cmd/web` need the `//go:build js && wasm` constraint (they import `syscall/js`, which only exists under `GOOS=js`) so native `go build ./...` / `go vet ./...` skip them.

`web/app.wasm` and `web/wasm_exec.js` are generated (gitignored); regenerate with `make build-web`.
