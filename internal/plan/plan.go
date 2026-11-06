// Package plan classifies source/target/state differences without writing.
package plan

import (
	"sort"

	"github.com/marvelousu/game-l10n-updater/internal/hashx"
	"github.com/marvelousu/game-l10n-updater/internal/model"
)

// Action is one deterministic plan outcome.
type Action string

const (
	ActionNew         Action = "new"
	ActionChanged     Action = "changed"
	ActionUnchanged   Action = "unchanged"
	ActionDeleted     Action = "deleted"
	ActionNeedsReview Action = "needs_review"
)

// AllActions is the fixed action order used in reports.
var AllActions = []Action{ActionNew, ActionChanged, ActionUnchanged, ActionDeleted, ActionNeedsReview}

// Decision is one key/locale classification.
type Decision struct {
	Key          string
	Locale       string
	Action       Action
	Reason       string
	Entry        model.SourceEntry
	SourceHash   string
	TargetText   string
	TargetHash   string
	HasTarget    bool
	Edited       bool
	Manual       bool
	Approved     bool
	Locked       bool
	NeedsReview  bool
	StateExisted bool
}

// Result contains decisions and counts.
type Result struct {
	Decisions   []Decision
	Counts      map[Action]int
	APIRequests int
}

// Build classifies every source and target key for every configured locale.
func Build(cat *model.SourceCatalog, targets map[string]map[string]string, st *model.State, locales []string) Result {
	byKey := map[string]model.SourceEntry{}
	for _, entry := range cat.Entries {
		byKey[entry.Key] = entry
	}
	locales = append([]string(nil), locales...)
	sort.Strings(locales)

	result := Result{Counts: map[Action]int{}}
	for _, action := range AllActions {
		result.Counts[action] = 0
	}
	for _, locale := range locales {
		targetMap := targets[locale]
		if targetMap == nil {
			targetMap = map[string]string{}
		}
		for _, entry := range cat.Entries {
			text, hasTarget := targetMap[entry.Key]
			es := st.Entry(locale, entry.Key)
			decision := classify(entry, text, hasTarget, es)
			decision.Locale = locale
			result.Decisions = append(result.Decisions, decision)
		}
		for key, text := range targetMap {
			if _, ok := byKey[key]; ok {
				continue
			}
			es := st.Entry(locale, key)
			d := Decision{
				Key:          key,
				Locale:       locale,
				Action:       ActionDeleted,
				Reason:       "key is not in the source catalog",
				TargetText:   text,
				TargetHash:   hashx.Text(text),
				HasTarget:    true,
				StateExisted: es != nil,
			}
			if es != nil {
				d.Manual = es.Manual
				d.Approved = es.Approved
				d.Locked = es.Locked
				d.NeedsReview = es.NeedsReview
			}
			result.Decisions = append(result.Decisions, d)
		}
	}

	sort.SliceStable(result.Decisions, func(i, j int) bool {
		if result.Decisions[i].Locale != result.Decisions[j].Locale {
			return result.Decisions[i].Locale < result.Decisions[j].Locale
		}
		return result.Decisions[i].Key < result.Decisions[j].Key
	})
	for _, d := range result.Decisions {
		result.Counts[d.Action]++
		if d.Action == ActionNew || d.Action == ActionChanged {
			result.APIRequests++
		}
	}
	return result
}

func classify(entry model.SourceEntry, targetText string, hasTarget bool, es *model.EntryState) Decision {
	sourceHash := hashx.Text(entry.Text)
	d := Decision{
		Key:        entry.Key,
		Entry:      entry,
		SourceHash: sourceHash,
		TargetText: targetText,
		HasTarget:  hasTarget,
		Edited:     hasTarget && es != nil && es.TargetHash != "" && hashx.Text(targetText) != es.TargetHash,
	}
	if hasTarget {
		d.TargetHash = hashx.Text(targetText)
	}
	if es != nil {
		d.StateExisted = true
		d.Manual = es.Manual
		d.Approved = es.Approved
		d.Locked = es.Locked
		d.NeedsReview = es.NeedsReview
	}

	if es == nil {
		if !hasTarget {
			d.Action = ActionNew
			d.Reason = "target is missing"
			return d
		}
		d.Action = ActionUnchanged
		d.Manual = true
		d.Reason = "existing target preserved as manual (no state)"
		return d
	}

	if !hasTarget {
		if es.Protected() || es.NeedsReview {
			d.Action = ActionNeedsReview
			d.NeedsReview = true
			d.Reason = "target is missing while the state is protected; not regenerated"
			return d
		}
		d.Action = ActionNew
		d.Reason = "target is missing"
		return d
	}

	if es.NeedsReview {
		d.Action = ActionNeedsReview
		d.Reason = "previously marked needs_review; approve or edit the target to resolve"
		return d
	}

	if es.SourceHash != sourceHash {
		if es.Protected() || d.Edited {
			d.Action = ActionNeedsReview
			d.NeedsReview = true
			if d.Edited && !es.Protected() {
				d.Manual = true
				d.Reason = "source changed and the target was manually edited; preserved for review"
			} else {
				d.Reason = "source changed; protected target preserved for review"
			}
			return d
		}
		d.Action = ActionChanged
		d.Reason = "source text changed"
		return d
	}

	d.Action = ActionUnchanged
	if d.Edited {
		d.Manual = true
		d.Reason = "manual target edit preserved"
	} else {
		d.Reason = "source and target unchanged"
	}
	return d
}
