package tools

import "github.com/brittonhayes/vala/internal/tool"

// Default builds the tool registry anchored at the given working directory:
// shell + file operations, native Sigma detection validation, and the Notion
// (ntn) integration.
//
// Future integrations (e.g. an aws tool for cloud investigation) register here
// too — that is the single extension point.
func Default(dir string) *tool.Registry {
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
		// Detection authoring: reference exemplars, validation, and a test runner.
		&ReferenceDetection{},
		&ValidateDetection{Dir: dir},
		&TestDetection{Dir: dir},
		// Surgical, comment-preserving field edits for Sigma rules.
		&SetDetectionMeta{Dir: dir},
		&SetDetectionLogsource{Dir: dir},
		&EditDetectionLogic{Dir: dir},
		&ManageDetectionList{Dir: dir},
		&SetDetectionRunbook{Dir: dir},
		&ManageDetectionTests{Dir: dir},
		// Notion documentation.
		&NTN{Dir: dir},
	)
	return r
}
