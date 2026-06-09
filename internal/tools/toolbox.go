package tools

import "github.com/brittonhayes/vala/internal/tool"

// Toolbox builds the single tool registry for vala's unified harness: the whole
// LEGO box of primitives the agent composes to hunt, record and link threat
// intelligence, author and validate detections, and work alerts. Every workflow
// that used to be its own command is now just a set of tools in this one box.
//
// It is the single extension point — register new integrations (e.g. an aws
// tool for cloud investigation) here. All tools are gated at call time by the
// permission gate (and, inside a governed case, by policy); exposing a tool here
// does not bypass that.
//
//   - dir       anchors the file/shell and detection-authoring tools.
//   - rc        is the session RunContext the hunt/intel tools write through;
//     open_hunt sets its active hunt at runtime.
//   - webhook   is the Slack webhook for the (gated) slack_notify action.
//   - cr        runs the governed response loop behind the open_case tool.
func Toolbox(dir string, rc *RunContext, webhook string, cr CaseRunner) *tool.Registry {
	r := tool.NewRegistry()
	r.Register(
		// Shell + file operations.
		&Bash{Dir: dir},
		&Read{Dir: dir},
		&Write{Dir: dir},
		&Edit{Dir: dir},
		&LS{Dir: dir},
		&Glob{Dir: dir},
		&Grep{Dir: dir},
		// Detection authoring: reference exemplars, validation, and a test runner,
		// plus surgical comment-preserving field edits for Sigma rules.
		&ReferenceDetection{},
		&ValidateDetection{Dir: dir},
		&TestDetection{Dir: dir},
		&SetDetectionMeta{Dir: dir},
		&SetDetectionLogsource{Dir: dir},
		&EditDetectionLogic{Dir: dir},
		&ManageDetectionList{Dir: dir},
		&SetDetectionRunbook{Dir: dir},
		&ManageDetectionTests{Dir: dir},
		// Investigation evidence source (mock-capable).
		&LogSearch{Dir: dir},
		// Hunting: recall what's already known, queue a trigger, open a hunt,
		// record findings and intel, link artifacts, store it.
		&Recall{RC: rc},
		&QueueHunt{RC: rc},
		&OpenHunt{RC: rc},
		&RecordFinding{RC: rc},
		&RecordIntel{RC: rc},
		&LinkArtifacts{RC: rc},
		&StoreHunt{RC: rc},
		// Incident response: the doorway into the governed case loop.
		&OpenCase{Runner: cr, Dir: dir},
		// Notion documentation.
		&NTN{Dir: dir},
	)
	return r
}
