//go:build js && wasm

package corpus

import (
	"net/http"
	"net/url"

	"github.com/ollama/ollama/api"
)

// defaultWasmHost is where Ollama listens by default. A browser has no
// environment to read OLLAMA_HOST from, so this is the fallback when the page
// does not pass ?host=.
const defaultWasmHost = "http://127.0.0.1:11434"

// newOllamaClient builds the API client for the wasm build.
//
// The http.Client here is deliberately NOT http.DefaultClient, and that is the
// whole reason this file exists. Under GOOS=js, net/http only sends a request
// through the browser's fetch() if the Transport has *no* dial hooks set; if
// any of Dial/DialContext/DialTLS/DialTLSContext is non-nil it honours that
// contract and dials instead, landing in Go's in-process fake network stack
// where a real connection to localhost can only ever fail with "connection
// refused" (see net/http/roundtrip_js.go).
//
// http.DefaultTransport *does* set DialContext. So handing the ollama client
// http.DefaultClient — which is what the native path does, correctly — means
// the browser never reaches the server at all. A zero-value Transport has no
// dial hooks, so the request goes out over fetch, which is the only way for
// wasm to reach the network.
func newOllamaClient(host string) (*api.Client, error) {
	if host == "" {
		host = defaultWasmHost
	}
	base, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	return api.NewClient(base, &http.Client{Transport: &http.Transport{}}), nil
}
