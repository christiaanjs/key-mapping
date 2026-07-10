package core

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
)

// LeftHandKeys enumerates the physical left-hand keys of the half-qwerty
// layout, in the fixed order used for Reference() rows: top row, home row,
// bottom row, number row.
//
// This is keyboard geometry — physical-layout data, not a mapping rule. It is
// used here to pick out which Karabiner manipulators describe the mirror
// layer, and it will also feed the future key-press simulator.
var LeftHandKeys = []rune{
	'q', 'w', 'e', 'r', 't',
	'a', 's', 'd', 'f', 'g',
	'z', 'x', 'c', 'v', 'b',
	'1', '2', '3', '4', '5',
}

// The subset of a Karabiner complex-modifications manipulator this parser
// cares about. Fields it doesn't use (to_if_alone, to_after_key_up, ...) are
// simply left out of the struct and ignored by json.Unmarshal.
type karabinerManipulatorFile struct {
	Manipulators []karabinerManipulator `json:"manipulators"`
}

type karabinerManipulator struct {
	Type       string               `json:"type"`
	Conditions []karabinerCondition `json:"conditions"`
	From       karabinerFrom        `json:"from"`
	To         []karabinerToEntry   `json:"to"`
}

type karabinerCondition struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value int    `json:"value"`
}

type karabinerFrom struct {
	KeyCode string `json:"key_code"`
}

// karabinerToEntry covers both `{"key_code": "..."}` and `{"set_variable":
// {...}}` shapes of a `to` entry. We only ever act on KeyCode, but SetVariable
// still has to be a field (even if unused) so json.Unmarshal doesn't choke —
// actually it wouldn't choke either way, this is just documentation of what a
// `to` entry may contain.
type karabinerToEntry struct {
	KeyCode string `json:"key_code"`
}

// ParseMapping reads every "*.json" manipulator file under dir in fsys and
// builds a Mapping from the alt-layer (space-held) mirror manipulators: a
// manipulator contributes a left-key -> output pair iff it has a condition
// {type: "variable_if", name: "alt", value: 1}, its from.key_code is one of
// LeftHandKeys, and its to is a single {key_code: ...} entry.
//
// fsys is never the OS filesystem directly — callers pass an embedded FS (see
// mappings.FS) so this stays wasm-safe. dir is the directory within fsys to
// scan (see mappings.Default).
func ParseMapping(fsys fs.FS, dir string) (Mapping, error) {
	matches, err := fs.Glob(fsys, path.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob %s/*.json: %w", dir, err)
	}
	sort.Strings(matches) // deterministic across filesystems

	leftSet := make(map[rune]bool, len(LeftHandKeys))
	for _, k := range LeftHandKeys {
		leftSet[k] = true
	}

	mirror := make(map[rune]rune)

	for _, name := range matches {
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}

		var mf karabinerManipulatorFile
		if err := json.Unmarshal(data, &mf); err != nil {
			return nil, fmt.Errorf("parse %s: %w", name, err)
		}

		for i, man := range mf.Manipulators {
			if !hasAltEngagedCondition(man.Conditions) {
				continue
			}

			leftKey, ok := keyCodeToRune(man.From.KeyCode)
			if !ok || !leftSet[leftKey] {
				continue
			}

			if len(man.To) != 1 || man.To[0].KeyCode == "" {
				continue
			}

			outKey, ok := keyCodeToRune(man.To[0].KeyCode)
			if !ok {
				return nil, fmt.Errorf(
					"%s: manipulator %d: unknown key_code %q as mirror output of %q",
					name, i, man.To[0].KeyCode, man.From.KeyCode,
				)
			}

			if prev, dup := mirror[leftKey]; dup && prev != outKey {
				return nil, fmt.Errorf(
					"%s: manipulator %d: left key %q has conflicting alt-mirror outputs %q and %q",
					name, i, string(leftKey), string(prev), string(outKey),
				)
			}
			mirror[leftKey] = outKey
		}
	}

	// Order the pairs by LeftHandKeys so Reference() has a stable, canonical
	// row order independent of file/manipulator scan order.
	pairs := make([]mirrorPair, 0, len(mirror))
	for _, left := range LeftHandKeys {
		if right, ok := mirror[left]; ok {
			pairs = append(pairs, mirrorPair{Left: left, Right: right})
		}
	}

	return newMirrorTable(pairs), nil
}

// hasAltEngagedCondition reports whether conds contains the alt-layer-engaged
// condition {type: "variable_if", name: "alt", value: 1}. A manipulator may
// carry additional conditions alongside it (e.g. nav.json's variable_unless on
// "nav") — those don't disqualify it.
func hasAltEngagedCondition(conds []karabinerCondition) bool {
	for _, c := range conds {
		if c.Type == "variable_if" && c.Name == "alt" && c.Value == 1 {
			return true
		}
	}
	return false
}

// keyCodeToRune converts a Karabiner key_code string to the rune it produces.
// Letters a-z and digits 0-9 map to themselves; a handful of named
// punctuation/space keys have their own entries. Reports ok=false for any
// key_code this parser doesn't recognize.
func keyCodeToRune(keyCode string) (rune, bool) {
	switch keyCode {
	case "semicolon":
		return ';', true
	case "slash":
		return '/', true
	case "period":
		return '.', true
	case "comma":
		return ',', true
	case "spacebar":
		return ' ', true
	}

	if len(keyCode) == 1 {
		r := rune(keyCode[0])
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r, true
		}
	}

	return 0, false
}
