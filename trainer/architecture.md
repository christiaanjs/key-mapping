# Architecture: Dual-Target Go App (Terminal + Browser)

## Overview

A single-user, local application with two frontends driven by one shared Go core:

- **Terminal UI** — a native binary built on Bubble Tea.
- **Web UI** — the same core compiled to WebAssembly, running entirely in the browser with no backend server.

The guiding principle is that the core owns all state and logic and knows nothing about how it is rendered. Each frontend is a thin adapter that translates platform events into core events and renders the core's state.

## Design principles

1. **Pure core.** The `core` package imports nothing platform-specific — no `os`, no `syscall/js`, no Bubble Tea. This is what allows it to compile for both a native binary and a WASM target.
2. **Event in, state out.** All mutation flows through a single `Dispatch(Event) State` entry point. Both frontends speak this vocabulary.
3. **Share the model, not the view.** Rendering is done separately for each target. Sharing views would force a lowest-common-denominator UI; a good terminal layout and a good web layout are rarely the same.
4. **No server.** The web build runs the core in the browser's WASM runtime. State lives in WASM memory; persistence, if needed, uses browser storage.

## Package layout

```
core/          pure Go — state, logic, Event/State types, Dispatch, Snapshot
cmd/tui/       Bubble Tea binary — imports core
cmd/web/       WASM entrypoint — imports core, uses syscall/js
web/           static assets — HTML, JS/Preact glue, wasm_exec.js
```

Two `main` packages import one `core`. Neither frontend imports the other.

## The core

The core exposes a small, rendering-agnostic API:

```go
type App struct { /* state */ }

func New() *App
func (a *App) Dispatch(ev Event) State
func (a *App) Snapshot() State
```

`Event` and `State` are plain data types. The core is event-driven so it maps cleanly onto both Bubble Tea's `Update` loop and the browser's event handlers.

## Terminal frontend

A normal Go binary. The Bubble Tea `Model` wraps `*App`:

- `Update` translates key/mouse messages into a core `Event`, calls `Dispatch`, and stores the returned state.
- `View` renders `Snapshot()` using Lip Gloss.

Bubble Tea's `Update` is single-goroutine, so no locking is needed on the terminal side.

## Web frontend

The web target compiles with `GOOS=js GOARCH=wasm` and imports only `core` — **not** Bubble Tea, which requires a TTY.

The WASM `main` registers the core on the JS global scope:

```go
func main() {
    app := core.New()
    js.Global().Set("dispatch", js.FuncOf(func(_ js.Value, args []js.Value) any {
        ev := parseEvent(args[0])   // JS object -> core.Event
        state := app.Dispatch(ev)
        return marshalState(state)  // core.State -> JSON for JS
    }))
    select {} // keep the runtime alive
}
```

A thin Preact layer calls `dispatch(...)` and renders the returned state. JavaScript handles only DOM and event wiring; all logic stays in Go.

### The JS boundary

Marshalling across `syscall/js` per field is clunky. The cleaner pattern is to serialize `Snapshot()` to JSON once per update and hand the whole blob to Preact to diff. Treat WASM as a local API that returns JSON — this mirrors the terminal's `Snapshot()` exactly.

## Data flow

```
             ┌──────────────┐
   key event │   cmd/tui    │ Lip Gloss render
  ───────────▶  Bubble Tea  ├──────────────▶ terminal
             │   Model      │
             └──────┬───────┘
                    │ Dispatch(Event) / Snapshot()
                    ▼
             ┌──────────────┐
             │     core     │  state + logic (pure)
             └──────▲───────┘
                    │ Dispatch(Event) / Snapshot()
             ┌──────┴───────┐
  DOM event  │   cmd/web    │ JSON state blob
  ───────────▶  WASM + js   ├──────────────▶ Preact -> browser
             │   glue       │
             └──────────────┘
```

## Cross-cutting concerns

### Binary size (web)

The standard Go WASM runtime is roughly 2 MB+ gzipped before any application code. If that is a concern, **TinyGo** reduces it substantially, but it does not support the full standard library or all reflection. Verify early that the core compiles under TinyGo, especially if it leans on `encoding/json`.

### Persistence

With no backend, state survives only as long as the WASM instance. If reload-persistence is needed, write `Snapshot()` to `localStorage` or `IndexedDB` via `syscall/js`. The terminal side can persist to a file if desired; both go through the same serialized state shape.

### Concurrency

Bubble Tea's `Update` is single-threaded. The browser event model is effectively single-threaded per WASM instance as well. Because this is a single-user local app with one core instance per frontend, no cross-goroutine locking of `App` is required. If that assumption ever changes (shared instance, background goroutines), guard `App` behind a mutex or give each session its own instance.

## Build

Terminal:

```
go build -o bin/app ./cmd/tui
```

Web:

```
GOOS=js GOARCH=wasm go build -o web/app.wasm ./cmd/web
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/
```

Serve the `web/` directory as static files (any static host or `file://`-capable setup); there is no application server.

## Summary

One pure core, two thin adapters. The terminal adapter renders with Lip Gloss inside Bubble Tea's loop; the web adapter runs the core in WASM and renders with Preact. Neither frontend knows about the other, and the web target needs no backend.
