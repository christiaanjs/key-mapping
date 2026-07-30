// Package core holds the pure, rendering-agnostic trainer state and logic.
//
// It must import nothing platform-specific (no os, no syscall/js, no Bubble
// Tea) so it compiles for both the native TUI binary and the GOOS=js
// GOARCH=wasm web target. See trainer/architecture.md.
package core
