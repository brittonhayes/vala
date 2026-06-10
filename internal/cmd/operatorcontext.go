package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/brittonhayes/vala/internal/agent"
)

// starterOperatorContext is the commented template vala writes for a new
// VALA.md. It teaches the operator which standing facts pay off most — the
// hunting analog of priming a coding agent with project context — so the file is
// useful the moment it is filled in.
const starterOperatorContext = `# vala operator context

<!--
This file is vala's standing memory. vala reads it before every session, so the
facts you record here prime every hunt — you never have to re-explain your
environment. Keep it short and high-signal, then delete these comments.
-->

## Environment
<!-- The estate you defend: cloud accounts, crown-jewel systems, key identities. -->

## Log sources
<!-- Where evidence lives, by behavior. e.g. "auth -> Okta system log; AWS API -> CloudTrail; network -> VPC flow logs". -->

## What normal looks like
<!-- Known-good baselines so the agent can tell weird from routine. e.g. "svc-deploy rotates keys nightly ~02:00 UTC". -->

## Conventions
<!-- Detection naming, severity scale, where rules live, who to escalate to. -->

## Prior incidents & standing hunches
<!-- Past incidents worth not re-litigating, and hunches worth hunting when there's time. -->
`

// scaffoldOperatorContext writes a starter VALA.md in cwd when none exists. It is
// best-effort and idempotent: an existing file is never overwritten, and a write
// failure is reported but does not fail the caller.
func scaffoldOperatorContext(cwd string) {
	path := filepath.Join(cwd, agent.OperatorContextFile)
	if _, err := os.Stat(path); err == nil {
		return // already present — never clobber operator memory
	}
	if err := os.WriteFile(path, []byte(starterOperatorContext), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "  (could not write %s: %v)\n", agent.OperatorContextFile, err)
		return
	}
	fmt.Fprintf(os.Stderr, "✓ wrote %s — fill it in so every hunt starts with context\n", agent.OperatorContextFile)
}
