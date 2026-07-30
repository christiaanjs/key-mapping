package core

// staticMapping is a TEMPORARY hardcoded stand-in for a future Karabiner-JSON parser.
// It implements the Mapping interface using mappings/qwerty-mirror/*.json.
// Once the parser lands, this will be replaced; the drill and frontends will see
// no change because they depend only on the interface, never on the concrete type.

type staticMapping struct {
	mirror      map[rune]rune // left key -> output
	reverseMap  map[rune]rune // output -> left key
	leftKeyList []rune        // left keys in order for Reference()
}

func newStaticMapping() Mapping {
	m := &staticMapping{
		mirror:      make(map[rune]rune),
		reverseMap:  make(map[rune]rune),
		leftKeyList: []rune{},
	}

	// Build the MIRROR table: physical left key -> produced output
	pairs := []struct {
		left  rune
		right rune
	}{
		{'q', 'p'}, {'w', 'o'}, {'e', 'i'}, {'r', 'u'}, {'t', 'y'},
		{'a', ';'}, {'s', 'l'}, {'d', 'k'}, {'f', 'j'}, {'g', 'h'},
		{'z', '/'}, {'x', '.'}, {'c', ','}, {'v', 'm'}, {'b', 'n'},
		{'1', '0'}, {'2', '9'}, {'3', '8'}, {'4', '7'}, {'5', '6'},
	}

	for _, p := range pairs {
		m.mirror[p.left] = p.right
		m.reverseMap[p.right] = p.left
		m.leftKeyList = append(m.leftKeyList, p.left)
	}

	return m
}

// Hint returns how to produce the given output character.
// Ports the prototype's routeFor() function.
func (m *staticMapping) Hint(output rune) KeyHint {
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
	if leftKey, exists := m.reverseMap[output]; exists {
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
func (m *staticMapping) Supported(output rune) bool {
	if output == ' ' {
		return true
	}
	if _, exists := m.mirror[output]; exists {
		return true
	}
	if _, exists := m.reverseMap[output]; exists {
		return true
	}
	return false
}

// Diagnose explains why typing got was wrong when want was expected.
// Ports the prototype's diagnose(T, R) function faithfully.
func (m *staticMapping) Diagnose(want, got rune) string {
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

	// user held space when they shouldn't have (rt.HoldSpace === false && MIRROR[want] === got)
	if !rt.HoldSpace {
		if mirrorResult, exists := m.mirror[want]; exists && mirrorResult == got {
			return "You held space (got \"" + string(got) + "\"). Release space, press \"" + string(want) + "\"."
		}
	}

	// user did wrong space combo (rt.HoldSpace === true && OUT_TO_LEFT[got] !== undefined)
	if rt.HoldSpace {
		if leftKeyForGot, exists := m.reverseMap[got]; exists {
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
func (m *staticMapping) Reference() []RefRow {
	rows := make([]RefRow, 0, len(m.leftKeyList))
	for _, leftKey := range m.leftKeyList {
		rows = append(rows, RefRow{
			Key:    string(leftKey),
			Output: string(m.mirror[leftKey]),
		})
	}
	return rows
}
