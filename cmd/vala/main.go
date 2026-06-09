// Command vala is an agentic security harness: it hunts threats against a
// hypothesis, stores hunts and threat intelligence in a Notion-backed brain,
// and authors and validates Sigma detections.
package main

import "github.com/brittonhayes/vala/internal/cmd"

func main() {
	cmd.Execute()
}
