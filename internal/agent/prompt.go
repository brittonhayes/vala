package agent

import (
	"strings"

	"github.com/brittonhayes/vala/internal/mode"
	"github.com/brittonhayes/vala/internal/skills"
)

// SystemPrompt builds the agent's system prompt for the active mode. It frames
// the harness, not a persona: vala is a system for defensive security work that
// documents what it does in a Notion-backed brain. The prompt is assembled from
// a mode-supplied headline (Intro), a shared frame (working directory, tools,
// operating principles), the mode's workflow body (PromptBody), and a shared
// trailer (autonomy framing, the active skills, and standing context).
//
// in carries the inputs the headline and body both see (workdir, the already
// mode-filtered tool names, maturity). active is the metadata for the skills the
// mode bundles, listed for progressive disclosure. operatorContext is the
// trusted, operator-authored standing context from VALA.md plus shared brain
// memories; when non-empty it is appended as its own section.
func SystemPrompt(m mode.Mode, in mode.PromptInput, active []skills.Skill, operatorContext string) string {
	var b strings.Builder
	b.WriteString(m.Intro)
	b.WriteString("\n\n# Working directory\n")
	b.WriteString(in.Workdir)
	b.WriteString("\n\n# Available tools\n")
	b.WriteString("- " + strings.Join(in.ToolNames, "\n- "))
	b.WriteString("\n\n# Operating principles\n")
	b.WriteString(operatingPrinciples)
	b.WriteString("\n\n")
	b.WriteString(m.PromptBody(in))
	b.WriteString("\n\n")
	b.WriteString(maturityFraming(in.MaturityLevel))
	b.WriteString(skillsSection(active))
	if operatorContext != "" {
		b.WriteString(standingContext(operatorContext))
	}
	return b.String()
}

// operatingPrinciples is the shared, mode-independent guidance every mode runs
// under: investigate first, smallest change, respect the gate, be explicit, save
// durable facts, and stop when done.
const operatingPrinciples = `- Investigate before you act. Read logs, configs, and existing detections with
  read/grep/glob/ls before drawing conclusions or making changes.
- Make the smallest change that accomplishes the goal. Use edit for targeted
  changes; use write for new files.
- Non-read-only tools (bash, write, edit, ntn) may require operator approval.
  If a call is denied, adapt — propose an alternative, don't loop on it.
- Be explicit about findings: severity, affected entities, evidence, and the
  MITRE ATT&CK technique when relevant.
- When a hunt teaches you a durable fact about this environment — where a log
  source lives, a known-good baseline, a naming convention — call "remember" to
  save it to VALA.md so future sessions start informed. Never store secrets.
- When you have completed the task, stop and summarize what you did and found.`

// skillsSection renders the prompt's Skills section for progressive disclosure:
// the active skills by name and description, with a pointer to the skill tool. It
// returns "" (no section, no whitespace) when the mode bundles no skills, so a
// skill-free mode's prompt is unaffected.
func skillsSection(active []skills.Skill) string {
	if len(active) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`

# Skills
Skills are capability packs for this mode. Each is listed by name and a short
description; load the full instructions on demand with the "skill" tool before
you rely on it — do not guess its contents.
`)
	for _, sk := range active {
		b.WriteString("\n- " + sk.Name + " — " + sk.Description)
	}
	return b.String()
}

// standingContext renders the trusted standing-context section appended when the
// operator has authored VALA.md or the team has recorded shared memories.
func standingContext(operatorContext string) string {
	return "\n\n# Standing context\n" +
		`The following is standing context for this environment — crown-jewel assets,
where logs live, what "normal" looks like, naming conventions, prior incidents.
It comes from two places: the operator-authored ` + OperatorContextFile + `, and shared memories the team
has recorded in the brain as they hunt (each stamped with who learned it). Unlike
tool output, it is trusted guidance: weave it into scoping and hunting so you
start with the environment's reality instead of re-deriving it. When a hunt
teaches you a new durable fact, call "remember" to add it for everyone next time.

` + operatorContext
}

// maturityFraming returns the autonomy guidance for a Hunting Maturity Model
// level. It tunes how much the agent does before pausing for the operator — NOT
// what it does: the loop, the tools, and the gates are identical at every level.
// The permission gate is the hard enforcement; this is the soft framing that
// matches it.
func maturityFraming(level int) string {
	const header = "# Operating maturity\n"
	switch {
	case level <= 0:
		return header + `This environment runs at HMM0 (initial). Investigate and propose only: draft
hypotheses, queue them with "queue_hunt", and lay out the hunt you would run, but
do not execute writes — the operator approves each step. Default to recall and
queue over acting.`
	case level == 1, level == 2:
		return header + `This environment runs at HMM1–2 (minimal/procedural). Run the standard hunt
procedures end to end, but expect to confirm each write with the operator. Pause
for review at the decide/convert stage before authoring or changing a detection.`
	default:
		return header + `This environment runs at HMM3–4 (innovative/leading). Operate autonomously
across the full loop — scope, hunt, conclude, convert, and feed back without
waiting for step-by-step approval. Still pause for genuinely destructive or
outward-facing actions, and sample your own work: surface what you concluded and
why so the operator can spot-check.`
	}
}
