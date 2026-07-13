package corpus

import (
	"reflect"
	"testing"
)

func TestWords(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "lowercases and splits punctuation",
			text: "The Quick, brown-fox! jumps.",
			want: []string{"the", "quick", "brown", "fox", "jumps"},
		},
		{
			name: "dedups preserving first-seen order",
			text: "cat dog cat bird dog",
			want: []string{"cat", "dog", "bird"},
		},
		{
			name: "digits act as separators",
			text: "item1 item2 abc123def",
			want: []string{"item", "abc", "def"},
		},
		{
			name: "empty text yields no words",
			text: "",
			want: nil,
		},
		{
			name: "only punctuation yields no words",
			text: "!!! --- ,,, 123",
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Words(tc.text)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Words(%q) = %#v, want %#v", tc.text, got, tc.want)
			}
		})
	}
}

func TestSentences(t *testing.T) {
	cases := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "keeps lines with at least 4 words, strips punctuation",
			text: "The quick, brown fox! jumps.\nhi\nShort one two\nA fine day for a walk outside.",
			want: []string{"the quick brown fox jumps", "a fine day for a walk outside"},
		},
		{
			name: "collapses internal whitespace",
			text: "the   quick    brown  fox   jumps",
			want: []string{"the quick brown fox jumps"},
		},
		{
			name: "dedups identical resulting sentences",
			text: "the quick brown fox jumps\nThe Quick Brown Fox Jumps!",
			want: []string{"the quick brown fox jumps"},
		},
		{
			name: "drops lines under 4 words",
			text: "hi there\na b c",
			want: nil,
		},
		{
			name: "digits are dropped, not treated as separators that add words",
			text: "room 101 has 4 windows today",
			want: []string{"room has windows today"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Sentences(tc.text)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Sentences(%q) = %#v, want %#v", tc.text, got, tc.want)
			}
		})
	}
}

func TestIsLowerAlpha(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"abc":   true,
		"Abc":   false,
		"ab1":   false,
		"ab c":  false,
		"z":     true,
		"hello": true,
	}
	for in, want := range cases {
		if got := isLowerAlpha(in); got != want {
			t.Errorf("isLowerAlpha(%q) = %v, want %v", in, got, want)
		}
	}
}
