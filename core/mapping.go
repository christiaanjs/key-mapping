package core

// KeyHint describes how to produce one output character on the mapped keyboard.
// It is the reverse lookup the drill uses to tell the user what to press next,
// mirroring the prototype's routeFor().
type KeyHint struct {
	Output    string `json:"output"`    // the character to produce ("" when none/complete)
	Key       string `json:"key"`       // the physical key to press
	HoldSpace bool   `json:"holdSpace"` // hold spacebar (engage the alt/mirror layer) while pressing Key
	IsSpace   bool   `json:"isSpace"`   // the target itself is a space: tap the spacebar
	Mapped    bool   `json:"mapped"`    // false when the character is not in the mapping (skip it)
}

// Mapping is the trainer's view of a keymap: what a key produces, how to produce
// a character, and how to explain mistakes. It is the seam that decouples the
// trainer from any concrete keymap source.
//
// staticMapping (newStaticMapping) is a hardcoded stand-in ported from the
// prototype's MIRROR table; ParseMapping (mapping_parse.go) builds the same
// shape of Mapping by parsing Karabiner JSON from mappings/. The drill and
// both frontends depend only on this interface, never on either concrete
// type — that is what let the real parser slot in unchanged.
type Mapping interface {
	// Hint returns how to produce the given output character. For an unmapped
	// character it returns KeyHint{Mapped: false}.
	Hint(output rune) KeyHint

	// Supported reports whether the mapping can produce output (space counts).
	Supported(output rune) bool

	// Diagnose explains, in one sentence, why typing got was wrong when want was
	// expected — the coaching text shown on a miss (see the prototype diagnose()).
	Diagnose(want, got rune) string

	// Reference returns the key -> output rows for the reference view: each
	// physical left-hand key and what holding space produces from it.
	Reference() []RefRow
}
