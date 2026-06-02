// Command vala is an agentic security detection & response harness: it
// drives an LLM agent that can investigate, author Sigma detection rules,
// operate shell/file tools, and document findings in Notion.
package main

import "github.com/brittonhayes/vala/internal/cmd"

func main() {
	cmd.Execute()
}
