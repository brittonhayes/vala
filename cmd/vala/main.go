// Command vala is an agentic security harness: it hunts threats against a
// hypothesis, stores hunts and threat intelligence in a Notion-backed brain,
// authors and validates Sigma detections, and works alerts through a governed
// response loop.
package main

import "github.com/brittonhayes/vala/internal/cmd"

func main() {
	cmd.Execute()
}
