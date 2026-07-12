# key-mapping

A local, single-user toolkit for **managing and practicing [Karabiner-Elements](https://karabiner-elements.pqrs.org/) key mappings**. The centerpiece is a **trainer** that drills you on a keymap, available as both a terminal app and a browser app driven by one shared Go core.

The current focus is a **left-hand-only layout** (Half-QWERTY): hold the spacebar to mirror the right-hand keys onto the left hand, with a `b`-held nav layer and `caps`/`tab` remaps. The mappings live in [`mappings/qwerty-mirror/`](mappings/qwerty-mirror).

> **Status:** Phases 0–5 of [`PLAN.md`](PLAN.md) are complete — the core, both frontends, the **mapping parser**, and the **pluggable corpus** all work and are covered by CI. The keymap is now parsed from `mappings/` (the hardcoded mapping is only a fallback), and practice content can come from a static bank, a text file, a codebase, or a local **Ollama** model. See [`PLAN.md`](PLAN.md) for the roadmap (6–7: simulation, LLM generation).

## Architecture

One **pure Go core** owns all state and logic; two thin frontends render it. The core imports nothing platform-specific, so it compiles for both a native binary and a `GOOS=js GOARCH=wasm` web target.

```
core/        pure Go — App with Dispatch(Event) State / Snapshot() State, the four
             drill modes, the Mapping / Corpus seam interfaces, and ParseMapping
corpus/      pluggable corpus sources (static, file, codebase, Ollama) — I/O lives
             here, outside the pure core
cmd/tui/     terminal frontend — Bubble Tea + Lip Gloss
cmd/web/     WASM entrypoint — exposes snapshot()/dispatch() over syscall/js
web/         static assets — HTML + a thin vanilla-JS renderer (no framework/CDN/build)
mappings/    the real Karabiner mapping JSON (parsed by ParseMapping) + an embed.FS
trainer/     architecture.md (design) + index.html (the original prototype, reference only)
```

All mutation flows through `Dispatch(Event) State`; both frontends speak only that vocabulary and render `Snapshot()`. The web build serializes the state to JSON once per update and lets JS render the blob. See [`trainer/architecture.md`](trainer/architecture.md) for the full design and [`CLAUDE.md`](CLAUDE.md) for working notes.

### The seams

The trainer runs off two interfaces, so implementations drop in without touching the drill or the frontends:

- **`Mapping`** (`core/mapping.go`) → parsed from `mappings/*.json` by `core.ParseMapping`, which builds the same `mirrorTable` as the `staticMapping` fallback (`core/mapping_static.go`) — so parsed and static behave identically by construction.
- **`Corpus`** (`core/corpus.go`) → a fixed bank (`corpus.Source`: static, file, codebase) or a `corpus.Stream` that generates text in the background, both falling back to `staticCorpus` (`core/corpus_static.go`).

`New(m, c)` injects both; `NewDefault()` wires the static ones. The frontends parse the embedded mapping and select a corpus, each falling back to the static implementation on error.

The one rule a `Corpus` must obey: **`Word`/`Sentence` are called on the frontend's event loop and must never block.** That is what lets a streaming source exist behind the same interface as a hardcoded list — it serves whatever it has and fetches in the background, rather than making the UI wait. A corpus reports its progress by optionally implementing `StatusReporter`, which the core surfaces as `State.Corpus` so frontends can show it.

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

### Corpus sources (terminal app)

By default the trainer **uses a local Ollama model if one is reachable, and the built-in static bank otherwise** — so it drills on fresh, generated text when you have Ollama running and needs no configuration when you don't. Choose explicitly with `-corpus`:

```sh
./bin/app                                       # auto: Ollama if reachable, else static (default)
./bin/app -corpus=static                        # force the built-in bank
./bin/app -corpus=file -corpus-path=text.txt    # words/sentences from a text file
./bin/app -corpus=code -corpus-path=.           # identifiers extracted from a codebase
./bin/app -corpus=ollama                        # force Ollama (report failure instead of downgrading)
```

Auto-detection probes the server (2s cap; an absent one refuses instantly, so there is no startup stall) and **resolves the model against what is actually pulled** — it will not pick the package default if you don't have it. `auto` degrades silently to the static bank; `-corpus=ollama` is the "I mean it" form, which surfaces a down server as a failure rather than quietly downgrading.

`-corpus=ollama` **streams** from a running [Ollama](https://ollama.com) server rather than fetching a batch up front. The app starts instantly on the static bank, generated text arrives in the background (usually within a second or two), and the trainer switches to it as soon as it lands. The status line shows exactly where you are:

```
corpus: ollama ⠹ generating (qwen3:8b) — drilling on static text meanwhile
corpus: ollama ● streaming (qwen3:8b) — 240 words, 60 sentences so far
corpus: ollama failed — drilling on static text; retrying. (connection refused)
```

Options:

- `-ollama-model` — model name (default `llama3.2`); it must be pulled first (`ollama pull llama3.2`).
- `-ollama-host` — server URL (default: `OLLAMA_HOST`, else `http://localhost:11434`).
- `-ollama-temperature` (default `1.0`) and `-ollama-repeat-penalty` (default `1.2`) — sampling. `0` uses these defaults; a **negative** value sends nothing and defers to the model's own declared parameters.

Those two defaults are measured, not guessed. What the corpus cares about is how many *distinct* usable items a round yields, because the buffer dedups and a round that adds nothing new triggers a backoff.

The lever that matters is **`repeat_penalty`, not `temperature`**. Many models — `qwen3` among them — declare `repeat_penalty 1`, i.e. no repetition penalty at all, and then repeat themselves when asked for a long list; raising temperature does not fix that. Averaged over 3 runs against `qwen3:8b` at `temperature 1.0`, asking for 60 words:

| `repeat_penalty` | distinct words | duplicates |
|---|---|---|
| 1.0 (off) | 57 | 33.5% |
| 1.1 | 76 | 21.6% |
| **1.2** | **82** | **2.6%** |
| 1.3 | 63 | 0.5% |

1.2 maximises distinct output while nearly eliminating duplicates. Don't push it higher: 1.3 suppresses duplicates further but the model produces less overall, and at 1.5 sentence yield drops sharply — the penalty starts suppressing the very words ordinary sentences are built from ("the", "a").

Content is **unbounded but memory is not**: words and sentences land in ring buffers (2000 / 500, oldest evicted), and the producer only wakes when the drill has actually consumed enough to need more — an idle trainer generates nothing, so it won't sit there burning your GPU. Reasoning models (qwen3, deepseek-r1, …) have thinking disabled automatically; otherwise they spend minutes emitting a chain-of-thought before producing a single usable word.

Any source that fails (missing file, unreachable Ollama, a model repeating itself) falls back to the static bank and keeps drilling — a dead Ollama server never breaks the trainer, it just retries with a backoff.

### The web app streams too

The browser build supports the same streaming corpus, with the same auto-detection, selected by query string (its equivalent of the TUI's flags):

```
http://localhost:8000/                                  auto: Ollama if reachable, else static
http://localhost:8000/?corpus=static                    force the built-in bank
http://localhost:8000/?corpus=ollama                    force Ollama
http://localhost:8000/?corpus=ollama&model=qwen3:8b     ...with a specific model
http://localhost:8000/?corpus=ollama&host=http://…      ...on a specific server
http://localhost:8000/?corpus=ollama&temperature=1.1    ...tuning sampling
```

The same sampling knobs are available as `temperature` and `repeat-penalty` query params.

The page shows the same status line, and starts instantly on static text while the model warms up. There is no `file`/`code` option — those need a filesystem the sandbox doesn't have.

This works despite the sandbox because **Ollama's default CORS policy allows `localhost` origins**, so a page served from `http://localhost:8000` may call `http://localhost:11434` directly. Serve over HTTP from localhost (`make serve-web`): under `file://` the origin is `null`, which Ollama rejects — and `instantiateStreaming` needs HTTP regardless.

To verify the browser build actually works (not just that it compiles):

```sh
make smoke-web          # boots web/app.wasm under Node, asserts the static bank serves
make smoke-web-ollama   # ...and that it really streams generated text from Ollama
```

To exercise the Ollama path against a real server:

```sh
OLLAMA_LIVE_TEST=1 OLLAMA_TEST_MODEL=qwen3:8b go test ./corpus -run TestFromOllamaLive -v
```

It is skipped by default (and in CI), since it needs a server and a pulled model.

## Development

CI (`.github/workflows/ci.yml`) runs gofmt + `go vet` + native and wasm builds + `go test` on every push.

Two things to know when touching the web target:

- Build the wasm with `-o` (`go build -o web/app.wasm ./cmd/web`) — a bare `go build ./cmd/web` emits a binary named `web` that collides with the `web/` directory.
- Files in `cmd/web` need the `//go:build js && wasm` constraint (they import `syscall/js`, which only exists under `GOOS=js`) so native `go build ./...` / `go vet ./...` skip them.

`web/app.wasm` and `web/wasm_exec.js` are generated (gitignored); regenerate with `make build-web`.
