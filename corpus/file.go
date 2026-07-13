package corpus

import (
	"io"
	"os"
)

// FromText builds a Source from raw text by tokenizing words and sentences
// out of it.
func FromText(text string) *Source {
	return &Source{
		words:     Words(text),
		sentences: Sentences(text),
	}
}

// FromReader builds a Source by reading all of r and tokenizing it.
func FromReader(r io.Reader) (*Source, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return FromText(string(data)), nil
}

// FromFile builds a Source from the contents of the file at path.
func FromFile(path string) (*Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return FromText(string(data)), nil
}
