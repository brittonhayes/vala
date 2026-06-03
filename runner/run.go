package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/governance"
	"github.com/brittonhayes/vala/internal/permission"
	"github.com/brittonhayes/vala/internal/policy"
	"github.com/brittonhayes/vala/internal/respond"
	"github.com/brittonhayes/vala/internal/tool"
	"github.com/brittonhayes/vala/internal/tools"
)

// Outcome is the result of replaying one fixture.
type Outcome struct {
	Name               string    `json:"name"`
	Pass               bool      `json:"pass"`
	Violations         []string  `json:"violations"`
	Denied             []string  `json:"denied"`
	Executed           []string  `json:"executed"`
	EvidenceCount      int       `json:"evidence_count"`
	CasePageViolations []string  `json:"case_page_violations"`
	Scores             Scorecard `json:"scores"`
}

// memNotifier records notifications without sending anything.
type memNotifier struct{ sent int }

func (m *memNotifier) Notify(string) (string, error) {
	m.sent++
	return fmt.Sprintf("mock:%d", m.sent), nil
}

// RunFixture replays one scenario through the governance machine and scores it.
func RunFixture(ctx context.Context, fx Fixture) Outcome {
	env := fx.Env
	if env == "" {
		env = "dev"
	}
	pol := policy.Default()
	gate := permission.New(permission.ModeAllow, nil)
	gate.Policy = pol

	store := brain.NewMem()
	bc := brain.New(store)
	caseID, _ := bc.OpenCase(ctx, fx.Alert, fx.Name)

	ledger := governance.NewLedger()
	rc := tools.NewRunContext(env, caseID, bc, ledger, pol)
	rc.Notifier = &memNotifier{}

	logs := fx.MockLogs
	reg := tool.NewRegistry()
	reg.Register(
		&tools.LogSearch{Search: func(string) []map[string]any { return logs }},
		&tools.RecordEvidence{RC: rc},
		&tools.ProposeAction{RC: rc},
		&tools.SubmitForApproval{RC: rc},
		&tools.WriteCasePage{RC: rc},
		&tools.SlackNotify{RC: rc},
		// Generic approval-required action so scenarios can exercise the
		// approval path without a real integration.
		mockAction{name: "github_issue"},
	)

	out := Outcome{Name: fx.Name, Pass: true}
	captures := map[string]string{} // capture name -> evidence id
	execCount := map[string]int{}

	for _, step := range fx.Steps {
		t, ok := reg.Get(step.Tool)
		if !ok {
			out.Violations = append(out.Violations, "unknown tool in step: "+step.Tool)
			continue
		}
		input := resolveCaptures(step.Input, captures)
		raw, _ := json.Marshal(input)

		// Schema validation dimension: required fields must be present.
		if missing := missingRequired(t, input); len(missing) > 0 {
			out.Scores.schemaInvalid = true
			out.Violations = append(out.Violations, fmt.Sprintf("step %s missing required fields %v", step.Tool, missing))
		}

		class := pol.ClassOf(step.Tool)
		req := governance.Request{
			Tool: step.Tool, Summary: step.Adversarial, ReadOnly: t.ReadOnly(),
			Phase: governance.Phase(step.Phase), Class: class, Env: env,
		}
		if class == governance.ClassActionExecute {
			req.ActionID = governance.ActionID(step.Tool, raw)
		}
		d := gate.Decide(req, ledger)
		if !d.Allow {
			out.Denied = appendUnique(out.Denied, step.Tool)
			// An action denied during investigation is correct behavior; record
			// that scope creep / injection was resisted.
			continue
		}
		res, err := t.Run(ctx, raw)
		if err != nil {
			out.Violations = append(out.Violations, fmt.Sprintf("step %s errored: %v", step.Tool, err))
			continue
		}
		if class == governance.ClassActionExecute && !res.IsError {
			execCount[step.Tool]++
		}
		if step.CaptureEvidenceAs != "" {
			ev := rc.Evidence()
			if len(ev) > 0 {
				captures[step.CaptureEvidenceAs] = ev[len(ev)-1].ID
			}
		}
	}

	// Deterministic approval + execution, identical to `vala respond`.
	eng := &respond.Engine{Gate: gate, Brain: bc, Policy: pol, Env: env}
	if len(fx.Approve) > 0 {
		approve := map[string]bool{}
		for _, name := range fx.Approve {
			approve[name] = true
		}
		eng.Approver = func(p governance.ProposedAction) bool { return approve[p.Tool] }
	}
	executed := eng.ApproveAndExecute(ctx, reg, ledger, rc)
	for _, id := range executed {
		for _, p := range ledger.Proposed() {
			if p.ID == id {
				execCount[p.Tool]++
				out.Executed = appendUnique(out.Executed, p.Tool)
			}
		}
	}

	out.EvidenceCount = len(rc.Evidence())

	// Case page lint (evidence-backed claims dimension).
	if fx.CasePage != nil {
		page := brain.CasePage{
			CaseID:       caseID,
			Summary:      resolveClaims(fx.CasePage.Summary, captures),
			Hypotheses:   resolveClaims(fx.CasePage.Hypotheses, captures),
			NextSteps:    fx.CasePage.NextSteps,
			EvidenceRows: rc.Evidence(),
			Actions:      rc.Actions(),
		}
		out.CasePageViolations = brain.LintCasePage(page)
	}

	out.Scores = score(fx, out, execCount)
	out.Violations = append(out.Violations, checkExpectations(fx, out, execCount)...)
	out.Pass = len(out.Violations) == 0
	return out
}

func resolveCaptures(in map[string]any, captures map[string]string) map[string]any {
	return resolveValue(in, captures).(map[string]any)
}

func resolveValue(v any, captures map[string]string) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = resolveValue(val, captures)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = resolveValue(val, captures)
		}
		return out
	case string:
		if strings.HasPrefix(t, "$") {
			if id, ok := captures[strings.TrimPrefix(t, "$")]; ok {
				return id
			}
		}
		return t
	default:
		return v
	}
}

// resolveClaims substitutes $capture tokens in claim evidence IDs.
func resolveClaims(claims []brain.Claim, captures map[string]string) []brain.Claim {
	out := make([]brain.Claim, len(claims))
	for i, c := range claims {
		ev := make([]string, len(c.Evidence))
		for j, id := range c.Evidence {
			if strings.HasPrefix(id, "$") {
				if real, ok := captures[strings.TrimPrefix(id, "$")]; ok {
					ev[j] = real
					continue
				}
			}
			ev[j] = id
		}
		c.Evidence = ev
		out[i] = c
	}
	return out
}

func missingRequired(t tool.Tool, input map[string]any) []string {
	var missing []string
	for _, req := range t.Schema().Required {
		if _, ok := input[req]; !ok {
			missing = append(missing, req)
		}
	}
	return missing
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
