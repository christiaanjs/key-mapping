.PHONY: build-tui build-web test vet

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
