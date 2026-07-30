package core

// Mode identifies which trainer screen is active. Frontends render one mode at
// a time and switch with an EvSwitchMode event.
type Mode string

const (
	ModeMirror    Mode = "mirror"    // typing drill (words/sentences) through the mapping
	ModeNav       Mode = "nav"       // arrow-sequence drill on the nav layer
	ModeScratch   Mode = "scratch"   // free typing against the live keymap
	ModeReference Mode = "reference" // static key -> output table
)

// Content selects what mirror mode drills.
type Content string

const (
	ContentWords     Content = "words"
	ContentSentences Content = "sentences"
)

// Length filters word selection in mirror mode (matches the prototype's chips).
type Length string

const (
	LengthShort Length = "short" // len <= 4
	LengthAny   Length = "any"
	LengthLong  Length = "long" // len >= 6
)

// EventType tags the Event union. Events are plain data so both the Bubble Tea
// frontend and the WASM/JS frontend can construct them identically.
type EventType string

const (
	EvSwitchMode EventType = "switch_mode" // -> Mode
	EvSetContent EventType = "set_content" // -> Content
	EvSetLength  EventType = "set_length"  // -> Length
	EvType       EventType = "type"        // a character arrived (mirror/scratch) -> Rune, AtMillis
	EvArrow      EventType = "arrow"       // an arrow arrived (nav) -> Arrow
	EvSkip       EventType = "skip"        // skip current drill item
)

// Arrow directions for the nav drill.
const (
	ArrowLeft  = "left"
	ArrowDown  = "down"
	ArrowUp    = "up"
	ArrowRight = "right"
)

// Event is the single input type. All mutation flows through App.Dispatch(Event).
//
// The core is pure and cannot read the clock, so timing-sensitive events carry
// AtMillis: a frontend-supplied Unix-millis timestamp used to compute WPM. This
// keeps the core free of any platform time import.
type Event struct {
	Type     EventType `json:"type"`
	Mode     Mode      `json:"mode,omitempty"`
	Content  Content   `json:"content,omitempty"`
	Length   Length    `json:"length,omitempty"`
	Rune     string    `json:"rune,omitempty"`  // the character that arrived (string, not rune, for clean JS/JSON)
	Arrow    string    `json:"arrow,omitempty"` // one of ArrowLeft..ArrowRight
	AtMillis int64     `json:"atMillis,omitempty"`
}

// CharStatus colours each character of the drill target as the user progresses.
type CharStatus string

const (
	CharDone    CharStatus = "done"    // already typed correctly
	CharCurrent CharStatus = "current" // the character to type next
	CharPending CharStatus = "pending" // not yet reached
)

// DrillChar is one character of the drill target plus its render status.
type DrillChar struct {
	Char   string     `json:"char"`
	Status CharStatus `json:"status"`
}

// Stats are the live counters shown in every drill mode.
type Stats struct {
	Hits     int `json:"hits"`
	Misses   int `json:"misses"`
	Accuracy int `json:"accuracy"` // percent, 0-100
	WPM      int `json:"wpm"`      // words-per-minute (chars/5 over elapsed)
}

// DrillState is the mirror-mode drill (words or sentences).
type DrillState struct {
	Text     string      `json:"text"`
	Chars    []DrillChar `json:"chars"`
	Index    int         `json:"index"`
	Hint     KeyHint     `json:"hint"`     // how to type the current character
	Feedback string      `json:"feedback"` // diagnosis on a miss, or a completion note
	Complete bool        `json:"complete"`
	Stats    Stats       `json:"stats"`
}

// NavState is the nav-layer arrow-sequence drill.
type NavState struct {
	Sequence []string `json:"sequence"` // ArrowLeft..ArrowRight
	Index    int      `json:"index"`
	Hits     int      `json:"hits"`
	Misses   int      `json:"misses"`
	Feedback string   `json:"feedback"`
}

// RefRow is one row of the reference table: a physical key and what the mapping
// produces from it (under the mirror/alt layer).
type RefRow struct {
	Key    string `json:"key"`
	Output string `json:"output"`
}

// State is the complete, render-agnostic snapshot both frontends consume. Only
// the sub-state for the active Mode is populated; the rest are zero.
type State struct {
	Mode      Mode        `json:"mode"`
	Content   Content     `json:"content"` // echoed so frontends can render the active chip
	Length    Length      `json:"length"`  // echoed for the active length chip
	Drill     *DrillState `json:"drill,omitempty"`
	Nav       *NavState   `json:"nav,omitempty"`
	Reference []RefRow    `json:"reference,omitempty"`
}
