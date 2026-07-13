# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this project is

A single-user, local toolkit for **managing and practicing Karabiner-Elements key mappings**. The centerpiece is the **trainer**, an app that drills the user on a keymap. A secondary goal (later) is to **simulate** mappings and measure their typing efficiency.

The current focus is a **left-hand-only layout** (Half-QWERTY: hold spacebar to mirror the right-hand keys onto the left hand, plus a `b`-held nav layer and caps/tab remaps). The design must generalize to other layouts later, so avoid baking left-hand assumptions into shared code.

## Current state — read this first

Phases 0–5 of `PLAN.md` are built: the pure `core` (now including the mapping parser), the terminal (TUI) frontend, the web (WASM) frontend, and the pluggable `corpus` package all work and are covered by CI. What each thing is:

- `core/` — the pure trainer: `App` with `Dispatch(Event) State` / `Snapshot() State`, the four drill modes, the `Mapping` / `Corpus` **seam interfaces**, and `ParseMapping`.
- `corpus/` — pluggable corpus sources: fixed banks (static, file, codebase) and a **streaming** `Stream` fed by a vendor-neutral `Producer` (Ollama today; Phase 7's Anthropic provider slots into the same seam). All corpus I/O (os, net/http) and all goroutines live here, **outside** the pure core.
- `cmd/tui/` — Bubble Tea + Lip Gloss frontend. `cmd/web/` + `web/` — WASM entrypoint plus a thin vanilla-JS renderer (no framework/CDN/build step).
- `trainer/architecture.md` — the architecture this follows. `trainer/index.html` — the original **throwaway prototype**; it is the behavior/visual reference only, do not extend it.
- `mappings/qwerty-mirror/*.json` — the **real Karabiner mapping**; see the format section below. `mappings/mappings.go` embeds it as an `embed.FS` (wasm-safe) with `mappings.Default` = `"qwerty-mirror"`.

**The mapping is now parsed; the corpus is pluggable — both still behind the same interfaces.** `core.ParseMapping(fsys fs.FS, dir string) (Mapping, error)` (`core/mapping_parse.go`) reads `mappings/*.json` and builds a `mirrorTable` (`core/mirror_table.go`) — the same type `NewStaticMapping()` (`core/mapping_static.go`) builds, so parsed and static behave identically by construction. `core/corpus_static.go` (`staticCorpus`) and `corpus.Source` (`corpus/*.go`) both implement `Corpus`. `New(m Mapping, c Corpus)` injects them; `NewDefault()` wires the static ones. Both frontends parse the embedded mapping and (TUI) select a corpus, each falling back to the static implementation with an stderr warning on error. The drill and both frontends still depend only on the interfaces.

Remaining goals: Phase 6 (simulation/efficiency) and Phase 7 (LLM content generation, provider-flexible) — see `PLAN.md`.

Key seams to preserve when extending: keep `core` pure (the parser takes an `fs.FS`, never `os`; corpus I/O stays in `corpus/`). The web frontend has no file/codebase corpus (no filesystem in the sandbox), but it **does** stream from Ollama — see the wasm section below.

### The Corpus contract (read before touching corpus code)

**`Word`/`Sentence` are called on the frontend's event loop (Bubble Tea's `Update`, the browser's single JS thread) and must never block.** This is what lets a corpus that generates text over the network sit behind the same interface as a hardcoded list: `corpus.Stream` serves whatever is buffered (falling back to the static bank while cold) and fetches in the background, rather than making the UI wait. A corpus is *internally* concurrent; the `App` itself stays single-threaded and unlocked.

Consequences worth knowing before changing `corpus/stream.go`:
- A corpus reports progress by optionally implementing `core.StatusReporter`; the core surfaces it as `State.Corpus` so frontends can show that content is still arriving. The TUI ticks to re-render while it changes — Bubble Tea only redraws on messages, so background arrivals are otherwise invisible.
- The producer must stay **demand-driven**: it sleeps unless woken by consumption or a backoff timer. Two things preserve that, and both have regression tests — a pending wake must not short-circuit a backoff window, and a round that succeeds but adds nothing new (a model repeating itself, deduped away by the ring) must back off exactly like a failure. Break either and an idle trainer regenerates forever at full GPU.
- Reasoning models (qwen3, deepseek-r1) must have thinking disabled — they emit their chain-of-thought to `GenerateResponse.Thinking`, not `.Response`, and will churn for minutes producing no usable words. `ollamaProducer.noThink` resolves the capability from the server once; it is conditional because sending `think` to a model that lacks the capability is rejected.
- **The default corpus is `auto`: Ollama when reachable, static otherwise** (`corpus.Detect`, both frontends). Two traps it exists to avoid: (1) the server may be up but the *package default model* (`llama3.2`) not pulled — Detect resolves the model against `client.List()` and picks one that actually exists, skipping embedding-only models; (2) it must never stall startup — an absent server refuses instantly, and `DetectTimeout` (2s) only bites on a server that listens but doesn't answer. `auto` degrades silently; an explicit `-corpus=ollama` / `?corpus=ollama` deliberately skips the probe so a down server is *reported*, not quietly downgraded.
- **Sampling is tuned for *distinct* output, and `repeat_penalty` matters far more than `temperature`.** The buffer dedups, so a round that adds nothing new is wasted (and backs off). Many models (qwen3 included) declare `repeat_penalty 1` — no penalty at all — and repeat themselves on long lists; temperature does not fix that. Averaged over 3 runs on `qwen3:8b` at temp 1.0, 60 words requested: penalty off → 57 distinct / 33.5% dupes; 1.1 → 76 / 21.6%; **1.2 → 82 / 2.6%**; 1.3 → 63 / 0.5% (fewer items overall). Hence the defaults (1.0 / 1.2). Do not raise it further — at 1.5 sentence yield drops sharply, since it suppresses the common words sentences are made of. **Single runs vary wildly** (the same setting gave 64 and 117 distinct on consecutive runs), so average before concluding anything here. Unlike `think`, these need no capability check: Ollama accepts them for every completion model.
- **The top-up trigger scales with the bank (`kindState.topUpDue`), it is not a fixed count.** A kind earns a refill once the drill has drawn from it about as many times as it holds — one full pass. It was a flat 100 draws, which meant the 20-sentence bank had to be cycled *five times* (every sentence seen five times) before one new sentence appeared: skipping felt like it did nothing, because it did nothing. Tying it to the bank size also self-throttles, since top-ups get rarer as the ring grows. Note a kind is only drawn from in the mode using it, so the sentence bank does not grow while you drill words — that is correct, not a bug.
- **Each kind (words, sentences) gets its own producer goroutine.** This is load-bearing, not decoration. A `Produce` call runs to completion, and a model asked for a batch of words can stream for minutes (mostly duplicates that dedup discards). With one shared producer, sentences were starved for that entire time — observed live as sentences stuck at 0 for >2 minutes while words trickled in. Independent loops (each with its own ring, backoff and wake) mean neither kind blocks the other. The low-water marks double as the per-request batch size, so keep them small for the same reason.

### NEVER do I/O in cmd/web's main() before registering the JS globals

`go.run()` hands control back to the page the moment Go blocks on **anything** async. So if `main()` does a `fetch` (e.g. probing for Ollama) before `js.Global().Set("snapshot", ...)`, the page calls `snapshot()` before it exists and dies with **`window.snapshot is not a function`** — a dead page, not a slow one. This actually shipped, and `go build` was perfectly happy with it.

The rule: **register `snapshot`/`dispatch` first, do all I/O behind them.** Anything that needs I/O to decide the corpus goes through `corpus.Deferred`, which serves the static bank immediately and swaps the real source in when it resolves (reporting `warming` meanwhile, so the frontends keep polling and notice the upgrade).

`scripts/wasm-smoke.cjs` calls `snapshot()` *immediately* after `go.run()`, exactly as the page does, and fails if it is missing — a harness that waits first would hide this entire class of bug (it did). `web/app.js` also waits for the globals defensively, so a regression costs a slow boot rather than a blank page.

## The wasm build and Ollama (findings — don't re-derive these)

The browser **can** stream from a local Ollama. Earlier docs claimed it couldn't; that was wrong. Three non-obvious things make it work, each of which cost real debugging:

1. **CORS is not a problem.** Ollama's default policy allows `localhost` origins, so a page on `http://localhost:8000` may call `http://localhost:11434` directly (verified with a preflight). It must be served over HTTP from localhost: `file://` is origin `null`, which Ollama rejects.

2. **`http.DefaultClient` silently cannot reach the network under `GOOS=js`.** `net/http` only routes through the browser's `fetch()` when the Transport has *no* dial hooks; if `Dial`/`DialContext`/... is set it honours that and dials, landing in Go's in-process **fake network**, where localhost always fails with "connection refused". `http.DefaultTransport` *does* set `DialContext`. Hence `corpus/ollama_client_wasm.go`, which hands the client a zero-value `&http.Transport{}` — that, and only that, is what makes the request go out over fetch. (See `net/http/roundtrip_js.go`.)

3. **Node deliberately disables fetch**, so it cannot verify the above by default: Go sets `jsFetchDisabled` when it detects Node (via `process.argv0`, go.dev/issue/57613) and falls back to the same fake network. `scripts/wasm-smoke.cjs` works around it by swapping in a cloned `process` — `argv0` is read-only *and* non-configurable, and a `Proxy` may not lie about such a property, so a clone is the only way.

`make smoke-web` / `make smoke-web-ollama` run that harness. Prefer them over trusting `go build ./cmd/web`: compiling proves nothing about whether the browser can actually talk to the model — finding (2) compiled perfectly and was completely broken.

**Known cost, accepted:** importing the official Ollama client into the wasm build takes `web/app.wasm` from ~3.6 MB to ~12 MB (1.0 → 3.2 MB gzipped). The bulk is dead weight in a browser — `ollama/auth` pulls in `golang.org/x/crypto/ssh` (blowfish, curve25519, poly1305), plus `crypto/tls`, `crypto/x509`, `log/slog`, `regexp`. Fine for local single-user use. If it ever matters, the fix is a lean wasm-only `Producer` that speaks `/api/generate` over plain HTTP+JSON, behind the same build tag as `ollama_client_wasm.go` — no change to `Stream` or anything above it.

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
