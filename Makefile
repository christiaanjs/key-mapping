.PHONY: build-tui build-web test vet smoke-web smoke-web-ollama serve-web

# Native terminal binary.
build-tui:
	go build -o bin/app ./cmd/tui

# WebAssembly binary plus the Go JS runtime shim.
build-web:
	GOOS=js GOARCH=wasm go build -o web/app.wasm ./cmd/web
	cp "$(shell go env GOROOT)/lib/wasm/wasm_exec.js" web/

test:
	go test ./...

vet:
	go vet ./...

# Serve the web app. file:// will not work: instantiateStreaming needs HTTP,
# and Ollama's CORS policy only allows localhost origins.
serve-web: build-web
	cd web && python3 -m http.server 8000

# Run the built wasm module outside a browser and assert it actually works.
# `go build` only proves it compiles; this drives snapshot()/dispatch() for real.
smoke-web: build-web
	node scripts/wasm-smoke.cjs

# Same, but against a live Ollama (needs a server + `ollama pull qwen3:8b`).
# Asserts the browser build really streams generated text.
smoke-web-ollama: build-web
	node scripts/wasm-smoke.cjs "?corpus=ollama&model=qwen3:8b"
