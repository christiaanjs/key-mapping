# Implementation Plan

A phased build of the Karabiner keymap trainer and toolkit. Each phase is independently shippable and leaves the repo in a working, tested state. Phases 0–4 deliver a usable trainer; 5–7 are the roadmap extensions.

## Status

- ✅ **Phase 0 — Scaffolding** — done
- ⏭️ **Phase 1 — Mapping parser** — **deferred, not skipped.** Rather than build the parser first, Phase 2 shipped a hardcoded `staticMapping` (and `staticCorpus`) *behind the `Mapping`/`Corpus` interfaces*, so the trainer runs today. Phase 1 now means: implement a parser that drops in behind the existing `Mapping` interface. This is the recommended next step.
- ✅ **Phase 2 — Trainer core** — done (with the injected-static seam above)
- ✅ **Phase 3 — Terminal frontend** — done
- ✅ **Phase 4 — Web frontend** — done (vanilla JS instead of Preact; see note in that phase)
- ⬜ **Phases 5–7** — not started

Guiding constraints (from `trainer/architecture.md` and `CLAUDE.md`):
- The `core` package stays pure — no `os`, `syscall/js`, or Bubble Tea imports.
- Rules ultimately come from **parsing `mappings/`** (Phase 1), never permanently hardcoded — the hardcoded `staticMapping` is a temporary stand-in behind the `Mapping` interface.
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

> **Status: ⏭️ deferred (recommended next).** The `Mapping` interface and a hardcoded `staticMapping` behind it already exist (`core/mapping.go`, `core/mapping_static.go`). This phase replaces `staticMapping` with a parser-backed implementation — no changes to the drill or frontends.

**Goal:** turn the `mappings/` directory into a normalized, queryable model. This is the piece that replaces the prototype's hardcoded `MIRROR`/`routeFor`/`diagnose`.

- Model the Karabiner subset actually used: `basic` manipulators, `from`/`to` key codes, `set_variable`, `variable_if`/`variable_unless` conditions, `to_if_alone`, `to_after_key_up`.
- Represent **layers as variable states** (`alt`, `nav`, …) rather than assuming specific keys. Load a whole directory and merge multiple files into one mapping.
- Expose two lookups the trainer needs:
  - forward: `(physical keystroke, layer state) → output`
  - reverse: `output char → the keystroke + layer state that produces it` (for hints).
- Detect layer-arming keys (e.g. hold `spacebar` → `alt`, hold `b` → `nav`) and tap-vs-hold behavior.

**Key files:** `core/mapping/` (parser, model, lookup).

**Done when:** parsing `mappings/qwerty-mirror/` reproduces every rule the prototype hardcodes (mirror pairs, digit pairs, nav arrows, caps/tab), verified by table-driven tests against the JSON. No layout constants in code.

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

**Goal:** pluggable practice content, replacing the inline word/sentence banks.

- Define a corpus `Source` interface (yields words/sentences/streams) consumed by the trainer (and later the simulator).
- Implementations: static bank (migrate the prototype's list), plaintext file, and **codebase extraction** (tokenize source into practice text, filter to chars the mapping supports).
- Wire the trainer's content injection point (Phase 2) to a selected source.

**Key files:** `core/corpus/`.

**Done when:** the trainer can drill on text extracted from a real codebase, filtered to mappable characters.

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

- Define a `Generator` interface decoupled from any vendor.
- Implementations: Anthropic (see the `claude-api` skill for current model IDs/params) and a local-model backend; select via config, keep keys out of the repo.
- Feed generated content through the corpus `Source` interface (Phase 5) so the trainer consumes it uniformly.

**Key files:** `core/llm/` (or `content/`).

**Done when:** the trainer can drill on freshly generated content from at least one provider, swappable by config.

---

## Sequencing notes

- **1 → 2 → 3** is the critical path to a usable trainer; 4 reuses the same core.
- 5 unblocks richer content for 2/3/4 and is a prerequisite input for 6 and 7.
- 6 and 7 are independent of each other; both depend on 1 (and 5 for content).
