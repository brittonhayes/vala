package cmd

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/brittonhayes/vala/internal/agent"
	"github.com/brittonhayes/vala/internal/brain"
)

// teamMemoryLimit bounds how many shared memories are loaded into a session's
// standing context — recent enough to prime a hunt without flooding the prompt.
const teamMemoryLimit = 25

// sessionContext assembles the trusted standing context a session opens with:
// the operator-authored VALA.md plus the team's shared memories recalled from
// the brain. Either part may be empty; the whole thing is empty when there is
// nothing to say, and a brain read failure degrades to just VALA.md rather than
// blocking startup.
func sessionContext(ctx context.Context, cwd string, b *brain.Client) string {
	var parts []string
	if vala := agent.LoadOperatorContext(cwd); vala != "" {
		parts = append(parts, "## From VALA.md\n\n"+vala)
	}
	if b != nil {
		if mems, err := b.Memories(ctx, "", teamMemoryLimit); err == nil {
			if rendered := renderMemories(mems); rendered != "" {
				parts = append(parts, "## Team memory (shared brain)\n\n"+rendered)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

// renderMemories formats shared memories as attributed bullets, skipping any
// that carry no fact.
func renderMemories(mems []brain.Memory) string {
	var b strings.Builder
	for _, m := range mems {
		fact := strings.TrimSpace(m.Fact)
		if fact == "" {
			continue
		}
		if author := strings.TrimSpace(m.Author); author != "" {
			fmt.Fprintf(&b, "- (%s) %s\n", author, fact)
		} else {
			fmt.Fprintf(&b, "- %s\n", fact)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// resolveAuthor identifies the operator a session runs as, for stamping shared
// memories: an explicit VALA_AUTHOR wins, else the OS username, else "unknown".
func resolveAuthor() string {
	if a := strings.TrimSpace(os.Getenv("VALA_AUTHOR")); a != "" {
		return a
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "unknown"
}

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
