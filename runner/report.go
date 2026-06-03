package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Report aggregates the outcomes of a harness run.
type Report struct {
	Commit    string    `json:"commit"`
	Total     int       `json:"total"`
	Passed    int       `json:"passed"`
	Failed    int       `json:"failed"`
	Aggregate Scorecard `json:"aggregate"`
	Scenarios []Outcome `json:"scenarios"`
}

// RunAll replays every fixture and builds a report.
func RunAll(ctx context.Context, fixtures []Fixture, commit string) Report {
	r := Report{Commit: commit, Total: len(fixtures)}
	var sum Scorecard
	for _, fx := range fixtures {
		o := RunFixture(ctx, fx)
		r.Scenarios = append(r.Scenarios, o)
		if o.Pass {
			r.Passed++
		} else {
			r.Failed++
		}
		sum.ApprovalCompliance += o.Scores.ApprovalCompliance
		sum.NoScopeCreep += o.Scores.NoScopeCreep
		sum.EvidenceBacked += o.Scores.EvidenceBacked
		sum.InjectionResistance += o.Scores.InjectionResistance
		sum.SchemaValid += o.Scores.SchemaValid
	}
	if n := float64(max(1, r.Total)); n > 0 {
		r.Aggregate = Scorecard{
			ApprovalCompliance:  sum.ApprovalCompliance / n,
			NoScopeCreep:        sum.NoScopeCreep / n,
			EvidenceBacked:      sum.EvidenceBacked / n,
			InjectionResistance: sum.InjectionResistance / n,
			SchemaValid:         sum.SchemaValid / n,
		}
	}
	return r
}

// Text renders a human-readable report.
func (r Report) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Harness run @ %s\n", shortCommit(r.Commit))
	for _, o := range r.Scenarios {
		status := "PASS"
		if !o.Pass {
			status = "FAIL"
		}
		fmt.Fprintf(&b, "  [%s] %s\n", status, o.Name)
		for _, v := range o.Violations {
			fmt.Fprintf(&b, "        - %s\n", v)
		}
	}
	a := r.Aggregate
	fmt.Fprintf(&b, "Scorecards (avg): approval=%.2f scope=%.2f evidence=%.2f injection=%.2f schema=%.2f\n",
		a.ApprovalCompliance, a.NoScopeCreep, a.EvidenceBacked, a.InjectionResistance, a.SchemaValid)
	fmt.Fprintf(&b, "%d passed, %d failed of %d\n", r.Passed, r.Failed, r.Total)
	return b.String()
}

// JSON renders the report as indented JSON.
func (r Report) JSON() []byte {
	out, _ := json.MarshalIndent(r, "", "  ")
	return out
}

// Diff compares this report to a previous one and returns regression messages:
// scenarios that previously passed and now fail, or aggregate dimensions that
// dropped. An empty slice means no regressions.
func (r Report) Diff(prev Report) []string {
	var regressions []string
	prevPass := map[string]bool{}
	for _, o := range prev.Scenarios {
		prevPass[o.Name] = o.Pass
	}
	for _, o := range r.Scenarios {
		if was, ok := prevPass[o.Name]; ok && was && !o.Pass {
			regressions = append(regressions, "scenario regressed: "+o.Name)
		}
	}
	cmp := func(name string, now, before float64) {
		if now < before-1e-9 {
			regressions = append(regressions, fmt.Sprintf("%s dropped %.2f -> %.2f", name, before, now))
		}
	}
	cmp("approval_compliance", r.Aggregate.ApprovalCompliance, prev.Aggregate.ApprovalCompliance)
	cmp("no_scope_creep", r.Aggregate.NoScopeCreep, prev.Aggregate.NoScopeCreep)
	cmp("evidence_backed", r.Aggregate.EvidenceBacked, prev.Aggregate.EvidenceBacked)
	cmp("injection_resistance", r.Aggregate.InjectionResistance, prev.Aggregate.InjectionResistance)
	cmp("schema_valid", r.Aggregate.SchemaValid, prev.Aggregate.SchemaValid)
	return regressions
}

func shortCommit(c string) string {
	if len(c) >= 7 {
		return c[:7]
	}
	if c == "" {
		return "(unknown)"
	}
	return c
}
