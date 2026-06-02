package reference

import (
	"testing"

	"github.com/brittonhayes/vala/internal/detect"
)

// TestReferencesValid guards the gold standard: every embedded reference must
// pass the Sigma schema check.
func TestReferencesValid(t *testing.T) {
	files, err := Files()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no embedded reference detections found")
	}
	for path, data := range files {
		issues, err := detect.ValidateBytes(data)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		for _, is := range issues {
			t.Errorf("%s is invalid: %s", path, is.String())
		}
	}
}

// TestReferenceInlineTestsPass runs each reference's embedded `tests:` through
// the evaluation engine: the exemplars must actually do what they claim.
func TestReferenceInlineTestsPass(t *testing.T) {
	files, err := Files()
	if err != nil {
		t.Fatal(err)
	}
	for path, data := range files {
		results, err := detect.TestBytes(data)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		for _, r := range results {
			if r.Err != "" {
				t.Errorf("%s: %s", path, r.Err)
				continue
			}
			for _, c := range r.Cases {
				if !c.Passed() {
					t.Errorf("%s: case %q want match=%v got=%v err=%q",
						path, c.Name, c.Want, c.Got, c.Err)
				}
			}
		}
	}
}

// TestListAndGet exercises the public surface used by the tool.
func TestListAndGet(t *testing.T) {
	metas, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) == 0 {
		t.Fatal("List returned no references")
	}
	for _, m := range metas {
		if m.Title == "" {
			t.Errorf("%s has no title", m.Name)
		}
		if _, err := Get(m.Name); err != nil {
			t.Errorf("Get(%q): %v", m.Name, err)
		}
	}
	if _, err := Get("does-not-exist"); err == nil {
		t.Error("expected error for unknown reference")
	}
}
