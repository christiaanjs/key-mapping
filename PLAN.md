# Implementation Plan

A phased build of the Karabiner keymap trainer and toolkit. Each phase is independently shippable and leaves the repo in a working, tested state. Phases 0–5 deliver a usable trainer with pluggable content; 6–7 are the roadmap extensions.

## Status

- ✅ **Phase 0 — Scaffolding** — done
- ✅ **Phase 1 — Mapping parser** — done. `core.ParseMapping` reads `mappings/*.json` and drops in behind the `Mapping` interface; both frontends now parse the embedded mapping (static fallback on error).
- ✅ **Phase 2 — Trainer core** — done (with the injected-static seam above)
- ✅ **Phase 3 — Terminal frontend** — done
- ✅ **Phase 4 — Web frontend** — done (vanilla JS instead of Preact; see note in that phase)
- ✅ **Phase 5 — Corpus subsystem** — done. `corpus/` package: static, plaintext file, codebase extraction, and **Ollama** local-model sources, selected in the TUI via `-corpus`.
- ⬜ **Phases 6–7** — not started

Guiding constraints (from `trainer/architecture.md` and `CLAUDE.md`):
- The `core` package stays pure — no `os`, `syscall/js`, or Bubble Tea imports. The parser takes an `fs.FS` (the embedded `mappings.FS`); corpus I/O (files, network) lives in the separate `corpus` package, outside `core`.
- Rules come from **parsing `mappings/`** — the hardcoded `staticMapping` is now only a fallback behind the `Mapping` interface.
- Keep the current left-hand layout from special-casing shared code; model layouts as data.

---

## Phase 0 — Scaffolding

**Goal:** a compiling, testable Go module with the target package layout.

- `go mod init` (module path, Go version).
- Create empty packages: `core/`, `cmd/tui/`, `cmd/web/`, `web/`.
- Add `Makefile` (or `justfile`) with `build-tui`, `build-web`, `test`, `vet`, `lint` targets wrapping the commands in `architecture.md`.
- Wire `go vet` + `gofmt` check; decide on a linter (`golangci-lint`).

**Done when:** `go build ./...` and `go test ./...` pass on an empty skeleton; both build targets produce artifacts.

---

## Phase 1 — Mapping parser (the foundation)

> **Status: ✅ done.** `core.ParseMapping(fsys fs.FS, dir string) (Mapping, error)` (`core/mapping_parse.go`) reads `mappings/*.json` and builds a `mirrorTable` (`core/mirror_table.go`) — the same type `NewStaticMapping` builds, so parsed and static behave identically by construction. Both frontends call it against the embedded `mappings.FS`, falling back to `NewStaticMapping()` on error.

**Goal:** turn the `mappings/` directory into a normalized, queryable model. This is the piece that replaces the prototype's hardcoded `MIRROR`/`routeFor`/`diagnose`.

- Models the Karabiner subset actually used: `manipulators`, `from`/`to` key codes, and `variable_if`/`variable_unless` conditions.
- Represents the alt-mirror **layer as a variable state** (`{type: variable_if, name: alt, value: 1}`) rather than assuming specific keys; loads a whole directory (globbed, sorted for determinism) and merges files.
- Extracts left-key → output mirror pairs (physical left key in `LeftHandKeys`, single `to.key_code`); `keyCodeToRune` maps key codes to runes. Errors on unknown key codes and on conflicting duplicate mappings.
- Kept **wasm-safe**: takes an `fs.FS` (the embedded `mappings.FS`), never the OS filesystem.

**Key files:** `core/mapping_parse.go`, `core/mirror_table.go`, `core/mapping_parse_test.go`, `mappings/mappings.go` (embed).

**Done when:** parsing `mappings/qwerty-mirror/` reproduces every rule the static mapping hardcodes — verified by oracle tests asserting parsed `Hint`/`Supported`/`Diagnose`/`Reference` equal the static mapping's, plus `fstest.MapFS` edge cases.

---

## Phase 2 — Trainer core (pure logic)

> **Status: ✅ done.** See `core/`. Content and mapping come from injected `Corpus`/`Mapping` interfaces; static implementations wired by `NewDefault()`.

**Goal:** the rendering-agnostic `App` with `Dispatch(Event) State` / `Snapshot() State`, driven by the parsed mapping.

- Define `Event` and `State` as plain data types.
- Drill state machine covering the prototype's modes: **mirror** (word + sentence), **nav** sequence, **scratchpad**, **reference** (table generated from the parsed mapping).
- Per-keystroke **hint** and mistake **diagnosis** derived from the mapping lookups, not hand-written strings.
- Stats: hits, misses, accuracy, live WPM.
- Content (words/sentences) comes from an injected source interface — a stub static bank for now, real source in Phase 5.

**Key files:** `core/app.go`, `core/event.go`, `core/state.go`, `core/drill/`.

**Done when:** core tests drive a full drill via `Dispatch` and assert `Snapshot()` transitions (hit advances, miss diagnoses, sentence completion, WPM). Zero platform imports; `go build GOOS=js GOARCH=wasm ./core` succeeds.

---

## Phase 3 — Terminal frontend

> **Status: ✅ done.** See `cmd/tui/` (Bubble Tea + Lip Gloss). Modes switch on F1–F4.

**Goal:** a real playable TUI.

- Bubble Tea `Model` wrapping `*core.App`; `Update` translates key msgs → `core.Event` → `Dispatch`; `View` renders `Snapshot()` with Lip Gloss.
- Render each mode; keep layout TUI-native (don't mirror the web view).

**Key files:** `cmd/tui/`.

**Done when:** `go run ./cmd/tui` runs the mirror + nav + scratchpad + reference modes against the parsed mapping.

---

## Phase 4 — Web frontend

> **Status: ✅ done.** See `cmd/web/` + `web/`. Implemented with a thin **vanilla-JS** renderer (no framework/CDN/build step) instead of Preact — thinner, offline, honours the "logic in Go" principle. `cmd/web/main.go` exposes `snapshot()`/`dispatch(eventJSON)`. TinyGo not attempted (binary ~3.4 MB); revisit if size matters.

**Goal:** the same core in the browser, no backend.

- `cmd/web` WASM `main` registers `dispatch`/`snapshot` on the JS global; serialize `Snapshot()` to JSON per update.
- Thin Preact layer in `web/` renders the JSON blob and wires DOM events; port the prototype's visual design.
- Copy `wasm_exec.js`; serve `web/` statically.
- **Verify TinyGo early** if binary size matters (watch `encoding/json`/reflection).
- Optional: persist `Snapshot()` to `localStorage`.

**Key files:** `cmd/web/main.go`, `web/*`.

**Done when:** building to `web/app.wasm` and serving `web/` gives a working trainer with parity to the prototype, backed by the shared core.

---

## Phase 5 — Corpus subsystem

> **Status: ✅ done.** The `corpus/` package provides fixed banks (`FromText`/`FromReader`/`FromFile`, `FromCodebase`) and a **streaming** `Stream` fed by a pluggable `Producer` (today: Ollama). The TUI selects one via `-corpus` (`static|file|code|ollama`); each falls back to the static corpus on failure.

**Goal:** pluggable practice content, replacing the inline word/sentence banks.

- `Source` yields words/sentences via the existing `core.Corpus` interface (deterministic-by-seed selection, ported from `staticCorpus`), so the trainer and later the simulator consume it unchanged.
- Implementations: static bank, plaintext file/reader/text, **codebase extraction** (`Words`/`Sentences` tokenizers, camel/identifier splitting, filtered to the chars the mapping supports), and **Ollama** local-model generation.
- **Generation streams; it is not fetched upfront.** `Producer` emits each item the moment its line completes, and `Stream` serves the static fallback until the first items land — so the app starts instantly and upgrades in place instead of blocking on the model. Buffers are rings (unbounded content, bounded memory) and the producer only wakes on demand, so an idle trainer generates nothing. Failures (dead server, a model repeating itself) are soft: it backs off, retries, and keeps drilling on the fallback.
- The corpus's live state is surfaced through `core` as `State.Corpus` (via the optional `StatusReporter` interface), so the frontends can *show* the user that content is still arriving rather than appearing to hang.
- The web frontend keeps the static corpus — a browser sandbox cannot reach a local Ollama server or the filesystem.

**Key files:** `corpus/source.go`, `corpus/tokenize.go`, `corpus/file.go`, `corpus/codebase.go`, `corpus/producer.go`, `corpus/stream.go`, `corpus/ollama.go`.

**Done when:** the trainer can drill on text extracted from a real codebase or streamed from a local Ollama model, filtered to mappable characters. ✅ (Verified live against `qwen3:8b`.)

---

## Phase 6 — Simulation & efficiency

**Goal:** measure a mapping's cost over a corpus and compare layouts.

- Replay a corpus through a parsed mapping (Phase 1) to compute keystroke count, layer switches, and hand/finger travel per the layout's key geometry.
- Report per-mapping metrics; support comparing two mappings on the same corpus.
- Expose results through the core so both frontends can show them (later).

**Key files:** `core/sim/`.

**Done when:** running a corpus through `qwerty-mirror` yields reproducible efficiency metrics, and two mappings can be compared.

---

## Phase 7 — LLM content generation

**Goal:** generate trainer content via a provider-flexible API.

- **The seam already exists:** `corpus.Producer` (Phase 5) is deliberately vendor-neutral — `Produce(ctx, kind, n, emit)`. Ollama implements it today; Anthropic slots in alongside with no change to `Stream`, the drill, or the frontends.
- Add an Anthropic-backed `Producer` (see the `claude-api` skill for current model IDs/params); select the provider via config and keep keys out of the repo.
- Streaming, backoff, ring buffering, fallback, and status reporting all come for free from `Stream`.

**Key files:** `corpus/anthropic.go` (new), alongside the existing `corpus/producer.go`.

**Done when:** the trainer can drill on freshly generated content from at least one provider, swappable by config.

---

## Sequencing notes

- **1 → 2 → 3** is the critical path to a usable trainer; 4 reuses the same core. ✅
- 5 unblocks richer content for 2/3/4 and is a prerequisite input for 6 and 7. ✅
- 6 and 7 are independent of each other; both depend on 1 (and 5 for content) — both now unblocked.
