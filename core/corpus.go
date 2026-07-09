package core

// Corpus supplies practice text for the drill. It is the seam that decouples the
// trainer from where content comes from.
//
// The Phase-2 implementation (staticCorpus, newStaticCorpus) is a hardcoded bank
// copied from the prototype's word list and sentence list. Later phases will add
// implementations that extract text from a codebase or generate it with a
// language model; because the drill depends only on this interface, they slot in
// without touching the drill logic.
//
// The core is pure and holds no random source, so selection is seeded by the
// caller: the App passes an incrementing counter as seed. An implementation must
// be deterministic in seed (same seed -> same result) so drills are testable.
type Corpus interface {
	// Word returns a practice word matching the length filter, chosen by seed.
	Word(length Length, seed int) string

	// Sentence returns a practice sentence, chosen by seed.
	Sentence(seed int) string
}
