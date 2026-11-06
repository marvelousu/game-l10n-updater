package plan

import "github.com/marvelousu/game-l10n-updater/internal/model"

// ActionJSON is the serializable action shape used by plan reports.
type ActionJSON struct {
	Key         string `json:"key"`
	Locale      string `json:"locale"`
	Action      string `json:"action"`
	Reason      string `json:"reason"`
	SourceText  string `json:"source_text,omitempty"`
	TargetText  string `json:"target_text,omitempty"`
	SourceHash  string `json:"source_hash,omitempty"`
	TargetHash  string `json:"target_hash,omitempty"`
	Manual      bool   `json:"manual,omitempty"`
	Approved    bool   `json:"approved,omitempty"`
	Locked      bool   `json:"locked,omitempty"`
	NeedsReview bool   `json:"needs_review,omitempty"`
}

// Report is the JSON written by gamel10n plan.
type Report struct {
	SchemaVersion  int            `json:"schema_version"`
	Kind           string         `json:"kind"`
	SourceLanguage string         `json:"source_language"`
	Locales        []string       `json:"locales"`
	Actions        []ActionJSON   `json:"actions"`
	Counts         map[string]int `json:"counts"`
	APIRequests    int            `json:"api_requests"`
}

// NewReport builds a deterministic plan report.
func NewReport(sourceLanguage string, locales []string, result Result) *Report {
	actions := make([]ActionJSON, 0, len(result.Decisions))
	for _, d := range result.Decisions {
		actions = append(actions, ActionJSON{
			Key: d.Key, Locale: d.Locale, Action: string(d.Action), Reason: d.Reason,
			SourceText: d.Entry.Text, TargetText: d.TargetText,
			SourceHash: d.SourceHash, TargetHash: d.TargetHash,
			Manual: d.Manual, Approved: d.Approved, Locked: d.Locked, NeedsReview: d.NeedsReview,
		})
	}
	counts := map[string]int{}
	for _, action := range AllActions {
		counts[string(action)] = result.Counts[action]
	}
	return &Report{
		SchemaVersion:  model.SchemaVersion,
		Kind:           "plan",
		SourceLanguage: sourceLanguage,
		Locales:        append([]string(nil), locales...),
		Actions:        actions,
		Counts:         counts,
		APIRequests:    result.APIRequests,
	}
}
