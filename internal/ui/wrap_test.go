package ui

import "testing"

// TestInputRows checks that soft-wrapped text reports the visual row count, not
// just the number of hard newlines — this is what grows the input box.
func TestInputRows(t *testing.T) {
	cases := []struct {
		name  string
		value string
		width int
		want  int
	}{
		{"empty", "", 10, 1},
		{"short", "hello", 10, 1},
		{"hard newlines", "a\nb\nc", 10, 3},
		{"wraps once", "alpha beta", 10, 2},
		{"wraps several", "aaaa bbbb cccc dddd eeee ffff", 10, 3},
		{"hard plus soft", "first line here\nsecond", 10, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inputRows(tc.value, tc.width); got != tc.want {
				t.Fatalf("inputRows(%q, %d) = %d, want %d", tc.value, tc.width, got, tc.want)
			}
		})
	}
}
