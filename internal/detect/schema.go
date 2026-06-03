// Package detect provides native, offline validation of Sigma detection rules
// against the official Sigma JSON schema. No external CLI (sigma-cli, yq,
// Python) is required — the schema is embedded and compiled into the binary.
package detect

import (
	"bytes"
	_ "embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// sigmaSchemaJSON is the official Sigma rule JSON schema (draft 2020-12),
// vendored from SigmaHQ/sigma-specification. It has no external $refs, so it
// compiles and validates fully offline.
//
//go:embed sigma.schema.json
var sigmaSchemaJSON []byte

var (
	compileOnce sync.Once
	compiled    *jsonschema.Schema
	compileErr  error
)

// schema returns the compiled Sigma schema, compiling it exactly once.
func schema() (*jsonschema.Schema, error) {
	compileOnce.Do(func() {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(sigmaSchemaJSON))
		if err != nil {
			compileErr = fmt.Errorf("parse embedded sigma schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		const url = "sigma.schema.json"
		if err := c.AddResource(url, doc); err != nil {
			compileErr = fmt.Errorf("add sigma schema resource: %w", err)
			return
		}
		s, err := c.Compile(url)
		if err != nil {
			compileErr = fmt.Errorf("compile sigma schema: %w", err)
			return
		}
		compiled = s
	})
	return compiled, compileErr
}
