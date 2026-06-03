package governance

// Request is everything the permission gate needs to decide a single tool call
// in the governed loop. It is assembled by the agent at the moment of a
// tool_use and passed to permission.Gate.Decide.
type Request struct {
	Tool     string    // tool name
	Summary  string    // one-line human summary, for prompts
	ReadOnly bool      // the tool's own ReadOnly() result
	Phase    Phase     // current phase of the run
	Class    ToolClass // resolved tool class
	ActionID string    // governance.ActionID(tool, input); "" for non-actions
	Env      string    // dev | prod
}

// Decision is the gate's verdict. Reason is a short, loggable explanation that
// is also surfaced to the model when a call is denied so it can adapt.
type Decision struct {
	Allow  bool
	Reason string
}
