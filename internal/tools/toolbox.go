package tools

import (
	"github.com/brittonhayes/vala/internal/skills"
	"github.com/brittonhayes/vala/internal/tool"
)

// Toolbox builds the single tool registry for vala's unified harness: the whole
// LEGO box of primitives the agent composes to hunt, record and link threat
// intelligence, and author and validate detections. Every workflow that used to
// be its own command is now just a set of tools in this one box.
//
// It is the single extension point — register new integrations (e.g. an aws
// tool for cloud investigation) here. All tools are gated at call time by the
// permission gate; exposing a tool here does not bypass that.
//
//   - dir       anchors the file/shell and detection-authoring tools.
//   - rc        is the session RunContext the hunt/intel tools write through;
//     open_hunt sets its active hunt at runtime.
//   - sk        is the discovered skill catalog the skill tool reads from. It may
//     be nil (the skill tool then reports no skills available).
//   - evidence  are the discovered MCP evidence tools (e.g. Scanner's query and
//     discovery tools) the agent investigates through.
//
// The skill tool is always registered; whether it is exposed to the model is a
// per-mode decision the agent enforces (only when the mode bundles skills).
func Toolbox(dir string, rc *RunContext, sk *skills.Set, evidence ...tool.Tool) *tool.Registry {
	r := tool.NewRegistry()
	// Investigation evidence sources, discovered from configured MCP servers.
	r.Register(evidence...)
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
		// Hunting: recall what's already known, queue a trigger, open a hunt,
		// validate the data, record findings and intel, link artifacts, store it,
		// and update the coverage map in feedback.
		&Recall{RC: rc},
		&QueueHunt{RC: rc},
		&OpenHunt{RC: rc},
		&ValidateData{RC: rc},
		&RecordFinding{RC: rc},
		&RecordIntel{RC: rc},
		&LinkArtifacts{RC: rc},
		&StoreHunt{RC: rc},
		&UpdateCoverage{RC: rc},
		// Shared memory: record durable environment facts to the brain so the
		// whole team's future sessions start informed.
		&Remember{RC: rc},
		// Notion documentation.
		&NTN{Dir: dir},
		// Skills: load a mode-bundled capability pack in full on demand.
		&Skill{Set: sk},
	)
	return r
}
