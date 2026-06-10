package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// OperatorContextFile is the operator-authored memory file vala loads into the
// system prompt before every session: the standing context a hunter would
// otherwise carry in their head — crown-jewel assets, the log-source map, what
// "normal" looks like, detection naming conventions, prior incidents. It is the
// hunting analog of the project context a coding agent reads before it starts.
const OperatorContextFile = "VALA.md"

// LoadOperatorContext reads operator memory and returns it ready to embed in the
// system prompt, or "" when none exists. Two locations are merged — global
// first, then project — so a user-wide baseline is augmented, not replaced, by
// the repository's own VALA.md:
//
//   - <user config dir>/vala/VALA.md — user-wide standing context
//   - <workdir>/VALA.md              — project-specific, version-controlled
//
// Unlike tool output, this file is operator-authored and is therefore trusted
// context, not untrusted data. Loading is best-effort: unreadable or absent
// files are skipped silently so a session is never blocked on operator memory.
func LoadOperatorContext(workdir string) string {
	var b strings.Builder
	if dir, err := os.UserConfigDir(); err == nil {
		appendContextFile(&b, filepath.Join(dir, "vala", OperatorContextFile))
	}
	if workdir != "" {
		appendContextFile(&b, filepath.Join(workdir, OperatorContextFile))
	}
	return strings.TrimSpace(b.String())
}

// appendContextFile appends a context file's trimmed contents to b when it
// exists and is non-empty, separating multiple files with a blank line.
func appendContextFile(b *strings.Builder, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString(content)
}
