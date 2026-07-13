package core

// staticMapping is a TEMPORARY hardcoded stand-in for a future Karabiner-JSON parser.
// It implements the Mapping interface using mappings/qwerty-mirror/*.json.
// Once the parser lands, this will be replaced; the drill and frontends will see
// no change because they depend only on the interface, never on the concrete type.
//
// Both this and the parser (ParseMapping, mapping_parse.go) build a mirrorTable
// (mirror_table.go) from a set of left-key -> output pairs, so the two stay
// behaviorally identical by construction.

// NewStaticMapping returns the hardcoded mapping. It is exported so frontends
// can fall back to it when ParseMapping fails (e.g. a malformed mapping set).
func NewStaticMapping() Mapping { return newStaticMapping() }

func newStaticMapping() Mapping {
	// The MIRROR table: physical left key -> produced output.
	pairs := []mirrorPair{
		{'q', 'p'}, {'w', 'o'}, {'e', 'i'}, {'r', 'u'}, {'t', 'y'},
		{'a', ';'}, {'s', 'l'}, {'d', 'k'}, {'f', 'j'}, {'g', 'h'},
		{'z', '/'}, {'x', '.'}, {'c', ','}, {'v', 'm'}, {'b', 'n'},
		{'1', '0'}, {'2', '9'}, {'3', '8'}, {'4', '7'}, {'5', '6'},
	}

	return newMirrorTable(pairs)
}
