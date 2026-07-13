package core

import (
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/christiaanswanepoel/key-mapping/mappings"
)

// runesToCheck is the set of output runes exercised against both the parsed
// and static mappings, chosen to cover letters, digits, space, and the
// punctuation the mirror layer actually produces.
func runesToCheck() []rune {
	var rs []rune
	for r := 'a'; r <= 'z'; r++ {
		rs = append(rs, r)
	}
	for r := 'A'; r <= 'Z'; r++ {
		rs = append(rs, r)
	}
	for r := '0'; r <= '9'; r++ {
		rs = append(rs, r)
	}
	rs = append(rs, ' ', ';', '/', '.', ',', '!')
	return rs
}

func TestParseMapping_MatchesStatic(t *testing.T) {
	parsed, err := ParseMapping(mappings.FS, mappings.Default)
	if err != nil {
		t.Fatalf("ParseMapping(%s) error: %v", mappings.Default, err)
	}
	static := newStaticMapping()

	for _, r := range runesToCheck() {
		if got, want := parsed.Hint(r), static.Hint(r); got != want {
			t.Errorf("Hint(%q): parsed = %+v, static = %+v", r, got, want)
		}
		if got, want := parsed.Supported(r), static.Supported(r); got != want {
			t.Errorf("Supported(%q): parsed = %v, static = %v", r, got, want)
		}
	}
}

func TestParseMapping_ReferenceMatchesStatic(t *testing.T) {
	parsed, err := ParseMapping(mappings.FS, mappings.Default)
	if err != nil {
		t.Fatalf("ParseMapping(%s) error: %v", mappings.Default, err)
	}
	static := newStaticMapping()

	gotRows := parsed.Reference()
	wantRows := static.Reference()
	if !reflect.DeepEqual(gotRows, wantRows) {
		t.Fatalf("Reference() mismatch:\n parsed = %+v\n static = %+v", gotRows, wantRows)
	}

	// Spot check the documented order/content: q->p first, 5->6 last.
	if len(wantRows) == 0 || wantRows[0] != (RefRow{Key: "q", Output: "p"}) {
		t.Fatalf("sanity check failed, wantRows[0] = %+v", wantRows[0])
	}
}

func TestParseMapping_DiagnoseMatchesStatic(t *testing.T) {
	parsed, err := ParseMapping(mappings.FS, mappings.Default)
	if err != nil {
		t.Fatalf("ParseMapping(%s) error: %v", mappings.Default, err)
	}
	static := newStaticMapping()

	tests := []struct {
		name string
		want rune
		got  rune
	}{
		{"expected a space", ' ', 'q'},
		{"typed left key without space", 'p', 'q'},   // MIRROR[q] == p
		{"held space when shouldn't have", 'q', 'p'}, // want left key q, got mirror result p
		{"wrong space combo", 'p', 'o'},              // held space + w (-> o) instead of space + q (-> p)
		{"generic unmapped", '!', 'z'},               // '!' isn't in the map
		{"generic mismatch", 'o', 'x'},               // want mirror result o (space+w), got unrelated x
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMsg := parsed.Diagnose(tt.want, tt.got)
			wantMsg := static.Diagnose(tt.want, tt.got)
			if gotMsg != wantMsg {
				t.Errorf("Diagnose(%q, %q):\n parsed = %q\n static = %q", tt.want, tt.got, gotMsg, wantMsg)
			}
		})
	}
}

// TestParseMapping_UnknownKeyCode ensures a mirror manipulator using a
// key_code this parser doesn't recognize fails loudly instead of being
// silently dropped.
func TestParseMapping_UnknownKeyCode(t *testing.T) {
	fsys := fstest.MapFS{
		"badset/weird.json": &fstest.MapFile{Data: []byte(`{
			"manipulators": [
				{
					"type": "basic",
					"conditions": [{"name": "alt", "type": "variable_if", "value": 1}],
					"from": {"key_code": "q"},
					"to": [{"key_code": "not_a_real_key_code"}]
				}
			]
		}`)},
	}

	if _, err := ParseMapping(fsys, "badset"); err == nil {
		t.Fatal("ParseMapping with unknown key_code: got nil error, want error")
	}
}

// TestParseMapping_IgnoresNonAltManipulators is a smaller sanity check that
// manipulators without the alt=1 condition (e.g. the nav-layer arrows, or the
// spacebar's own to_if_alone) don't leak into the mirror table.
func TestParseMapping_IgnoresNonAltManipulators(t *testing.T) {
	fsys := fstest.MapFS{
		"set/mixed.json": &fstest.MapFile{Data: []byte(`{
			"manipulators": [
				{
					"type": "basic",
					"from": {"key_code": "spacebar"},
					"to": [{"set_variable": {"name": "alt", "value": 1}}],
					"to_if_alone": [{"key_code": "spacebar"}]
				},
				{
					"type": "basic",
					"conditions": [{"name": "nav", "type": "variable_if", "value": 1}],
					"from": {"key_code": "a"},
					"to": [{"key_code": "left_arrow"}]
				},
				{
					"type": "basic",
					"conditions": [{"name": "alt", "type": "variable_if", "value": 1}],
					"from": {"key_code": "q"},
					"to": [{"key_code": "p"}]
				}
			]
		}`)},
	}

	m, err := ParseMapping(fsys, "set")
	if err != nil {
		t.Fatalf("ParseMapping error: %v", err)
	}

	if !m.Supported('p') {
		t.Error("expected 'p' (q's mirror output) to be supported")
	}
	if m.Supported('a') && m.Hint('a').HoldSpace {
		// 'a' should be a mapped left key with no output pair here since it
		// has no alt=1 mirror manipulator in this fixture; just confirm the
		// nav-layer arrow manipulator wasn't mistaken for a mirror pair.
		t.Error("nav-layer manipulator leaked into the mirror table")
	}
	rows := m.Reference()
	if len(rows) != 1 || rows[0] != (RefRow{Key: "q", Output: "p"}) {
		t.Errorf("Reference() = %+v, want exactly [{q p}]", rows)
	}
}
