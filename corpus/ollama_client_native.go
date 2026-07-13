//go:build !js

package corpus

import (
	"net/http"
	"net/url"

	"github.com/ollama/ollama/api"
)

// newOllamaClient builds the API client for a native binary. An empty host
// means "resolve it the way the ollama CLI does" — i.e. from OLLAMA_HOST, or
// the client's own default.
func newOllamaClient(host string) (*api.Client, error) {
	if host == "" {
		return api.ClientFromEnvironment()
	}
	base, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	return api.NewClient(base, http.DefaultClient), nil
}
