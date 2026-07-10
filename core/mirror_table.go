package core

// mirrorPair is one physical-left-key -> produced-output association used to
// build a mirrorTable.
type mirrorPair struct {
	Left  rune
	Right rune
}

// mirrorTable is the shared Mapping implementation: a left-hand physical key
// mirrored to a produced output character while space is held. Both the
// hardcoded staticMapping (mapping_static.go) and the JSON-parsed mapping
// (ParseMapping, mapping_parse.go) build one of these from their respective
// sources, so Hint/Supported/Diagnose/Reference behave identically no matter
// where the left-key -> output pairs came from.
type mirrorTable struct {
	mirror   map[rune]rune // left key -> output
	reverse  map[rune]rune // output -> left key
	leftKeys []rune        // left keys, in Reference() order
}

// newMirrorTable builds a mirrorTable from pairs, preserving their order for
// Reference().
func newMirrorTable(pairs []mirrorPair) *mirrorTable {
	t := &mirrorTable{
		mirror:   make(map[rune]rune, len(pairs)),
		reverse:  make(map[rune]rune, len(pairs)),
		leftKeys: make([]rune, 0, len(pairs)),
	}
	for _, p := range pairs {
		t.mirror[p.Left] = p.Right
		t.reverse[p.Right] = p.Left
		t.leftKeys = append(t.leftKeys, p.Left)
	}
	return t
}

// Hint returns how to produce the given output character.
// Ports the prototype's routeFor() function.
func (m *mirrorTable) Hint(output rune) KeyHint {
	// space character: tap the spacebar alone
	if output == ' ' {
		return KeyHint{
			Output:  " ",
			IsSpace: true,
			Mapped:  true,
		}
	}

	// output is a left key: press it directly (no space)
	if _, exists := m.mirror[output]; exists {
		return KeyHint{
			Output: string(output),
			Key:    string(output),
			Mapped: true,
		}
	}

	// output is a mirror result: hold space and press the left key
	if leftKey, exists := m.reverse[output]; exists {
		return KeyHint{
			Output:    string(output),
			Key:       string(leftKey),
			HoldSpace: true,
			Mapped:    true,
		}
	}

	// unmapped character
	return KeyHint{
		Output: string(output),
		Mapped: false,
	}
}

// Supported reports whether the mapping can produce output (space counts).
func (m *mirrorTable) Supported(output rune) bool {
	if output == ' ' {
		return true
	}
	if _, exists := m.mirror[output]; exists {
		return true
	}
	if _, exists := m.reverse[output]; exists {
		return true
	}
	return false
}

// Diagnose explains why typing got was wrong when want was expected.
// Ports the prototype's diagnose(T, R) function faithfully.
func (m *mirrorTable) Diagnose(want, got rune) string {
	// want is space
	if want == ' ' {
		return "Expected a space — tap the spacebar (don't hold)."
	}

	// want is not in the map
	rt := m.Hint(want)
	if !rt.Mapped {
		return "\"" + string(want) + "\" isn't in the map."
	}

	// user typed a left key without space (got is left key, MIRROR[got] === want)
	if mirrorResult, exists := m.mirror[got]; exists && mirrorResult == want {
		return "You typed \"" + string(got) + "\" without space. Hold space + \"" + string(got) + "\" → \"" + string(want) + "\"."
	}

	// user held space when they shouldn't have (rt.HoldSpace === false && OUT_TO_LEFT[want] === got)
	if !rt.HoldSpace {
		if mirrorResult, exists := m.mirror[want]; exists && mirrorResult == got {
			return "You held space (got \"" + string(got) + "\"). Release space, press \"" + string(want) + "\"."
		}
	}

	// user did wrong space combo (rt.HoldSpace === true && OUT_TO_LEFT[got] !== undefined)
	if rt.HoldSpace {
		if leftKeyForGot, exists := m.reverse[got]; exists {
			return "You did space + \"" + string(leftKeyForGot) + "\" (→ \"" + string(got) + "\"). Need space + \"" + string(rt.Key) + "\" (→ \"" + string(want) + "\")."
		}
	}

	// generic case
	spacePrefix := ""
	if rt.HoldSpace {
		spacePrefix = "space + "
	}
	return "Got \"" + string(got) + "\". For \"" + string(want) + "\" press " + spacePrefix + "\"" + string(rt.Key) + "\"."
}

// Reference returns the key -> output rows for the reference view.
func (m *mirrorTable) Reference() []RefRow {
	rows := make([]RefRow, 0, len(m.leftKeys))
	for _, leftKey := range m.leftKeys {
		rows = append(rows, RefRow{
			Key:    string(leftKey),
			Output: string(m.mirror[leftKey]),
		})
	}
	return rows
}
