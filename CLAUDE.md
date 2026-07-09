# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this project is

A single-user, local toolkit for **managing and practicing Karabiner-Elements key mappings**. The centerpiece is the **trainer**, an app that drills the user on a keymap. A secondary goal (later) is to **simulate** mappings and measure their typing efficiency.

The current focus is a **left-hand-only layout** (Half-QWERTY: hold spacebar to mirror the right-hand keys onto the left hand, plus a `b`-held nav layer and caps/tab remaps). The design must generalize to other layouts later, so avoid baking left-hand assumptions into shared code.

## Current state — read this first

This repo is **pre-implementation**. There is no Go code, no `go.mod`, and no build yet. What exists:

- `trainer/architecture.md` — the target Go architecture (the plan to build toward).
- `trainer/index.html` — a **throwaway prototype** of the trainer, self-contained HTML/JS. It **hardcodes** the keymap (see the `MIRROR` object) and the drill logic. Use it as a behavior reference for what the trainer should do; do not extend it.
- `mappings/qwerty-mirror/*.json` — the **real Karabiner mapping** the trainer must consume.

The central rebuild goal: the Go trainer must **parse the mapping files** to derive its rules, instead of hardcoding them the way the prototype does. `mappings/` is the source of truth; the trainer should render drills, hints, and the reference table from parsed manipulators.

## Target architecture (from `trainer/architecture.md`)

One **pure Go core**, two thin frontends — the core owns all state and logic and knows nothing about rendering.

```
core/        pure Go — state, logic, Event/State types. Imports NOTHING platform-specific
             (no os, no syscall/js, no Bubble Tea) so it compiles for both native and WASM.
cmd/tui/     Bubble Tea + Lip Gloss binary — imports core
cmd/web/     WASM entrypoint (GOOS=js GOARCH=wasm) — imports core, uses syscall/js
web/         static assets — HTML, Preact glue, wasm_exec.js (no backend server)
```

Non-negotiable rules when building this out:

- **Event in, state out.** All mutation flows through a single `Dispatch(Event) State`; `Snapshot() State` reads current state. Both frontends speak only this vocabulary.
- **Share the model, not the view.** Each frontend renders independently (Lip Gloss for TUI, Preact for web). Do not try to share rendering.
- **JS boundary = JSON.** The web build serializes `Snapshot()` to JSON once per update and hands the blob to Preact. Treat WASM as a local API returning JSON, mirroring the TUI's `Snapshot()`.
- **No server, no locking.** Web state lives in WASM memory (persist to `localStorage`/`IndexedDB` if needed). One core instance per frontend, single-threaded event loop — no mutex on `App` unless that assumption changes.
- **Verify TinyGo early** if web binary size matters — the standard Go WASM runtime is ~2 MB+ gzipped, and TinyGo lacks full stdlib/reflection (watch `encoding/json`).

## Build commands (once the Go code exists)

Terminal:
```
go build -o bin/app ./cmd/tui
```

Web:
```
GOOS=js GOARCH=wasm go build -o web/app.wasm ./cmd/web
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/
```
Then serve `web/` as static files — there is no application server.

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
