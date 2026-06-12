package mode

import "strings"

// DefaultID is the mode the harness runs in when none is configured — the full
// hunt loop, which reproduces vala's classic behavior.
const DefaultID = "hunt"

// builtins is the ordered set of modes shipped in the binary. New modes (dart,
// report-review) slot in by appending here. Order is the listing order in /mode.
func builtins() []Mode {
	return []Mode{
		huntMode(),
		detectMode(),
	}
}

// All returns every built-in mode in listing order (hunt first).
func All() []Mode { return builtins() }

// Get returns the mode with the given id. The bool is false for an unknown id so
// callers can fail with a clear message listing the valid ids (see IDs).
func Get(id string) (Mode, bool) {
	for _, m := range builtins() {
		if m.ID == id {
			return m, true
		}
	}
	return Mode{}, false
}

// Default returns the default mode (hunt).
func Default() Mode {
	m, _ := Get(DefaultID)
	return m
}

// IDs returns the valid mode ids joined for error messages, e.g. "hunt, detect".
func IDs() string {
	ids := make([]string, 0)
	for _, m := range builtins() {
		ids = append(ids, m.ID)
	}
	return strings.Join(ids, ", ")
}
