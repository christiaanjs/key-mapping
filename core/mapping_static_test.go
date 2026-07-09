package core

import "testing"

func TestStaticMapping_Hint(t *testing.T) {
	m := newStaticMapping()

	tests := []struct {
		name   string
		output rune
		want   KeyHint
	}{
		{
			name:   "space",
			output: ' ',
			want:   KeyHint{Output: " ", IsSpace: true, Mapped: true},
		},
		{
			name:   "left key",
			output: 'q',
			want:   KeyHint{Output: "q", Key: "q", HoldSpace: false, Mapped: true},
		},
		{
			name:   "mirror result",
			output: 'p',
			want:   KeyHint{Output: "p", Key: "q", HoldSpace: true, Mapped: true},
		},
		{
			name:   "unmapped",
			output: '!',
			want:   KeyHint{Output: "!", Mapped: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Hint(tt.output)
			if got != tt.want {
				t.Errorf("Hint(%q) = %+v, want %+v", tt.output, got, tt.want)
			}
		})
	}
}

func TestStaticMapping_Supported(t *testing.T) {
	m := newStaticMapping()

	tests := []struct {
		name   string
		output rune
		want   bool
	}{
		{"space", ' ', true},
		{"left key", 'q', true},
		{"mirror result", 'p', true},
		{"unmapped", '!', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.Supported(tt.output); got != tt.want {
				t.Errorf("Supported(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}

func TestStaticMapping_Reference(t *testing.T) {
	m := newStaticMapping()

	rows := m.Reference()
	if len(rows) != 20 {
		t.Fatalf("len(Reference()) = %d, want 20", len(rows))
	}

	want := RefRow{Key: "q", Output: "p"}
	if rows[0] != want {
		t.Errorf("rows[0] = %+v, want %+v", rows[0], want)
	}
}

func TestStaticMapping_Diagnose(t *testing.T) {
	m := newStaticMapping()

	tests := []struct {
		name    string
		want    rune
		got     rune
		wantMsg string
	}{
		{
			name:    "expected a space",
			want:    ' ',
			got:     'q',
			wantMsg: "Expected a space — tap the spacebar (don't hold).",
		},
		{
			name:    "typed left key without holding space",
			want:    'p', // mirror result of 'q'
			got:     'q', // typed the left key bare
			wantMsg: "You typed \"q\" without space. Hold space + \"q\" → \"p\".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Diagnose(tt.want, tt.got)
			if got != tt.wantMsg {
				t.Errorf("Diagnose(%q, %q) = %q, want %q", tt.want, tt.got, got, tt.wantMsg)
			}
		})
	}
}
