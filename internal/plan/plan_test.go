package plan

import (
	"reflect"
	"testing"

	"github.com/marvelousu/game-l10n-updater/internal/hashx"
	"github.com/marvelousu/game-l10n-updater/internal/model"
)

func fixture() (*model.SourceCatalog, map[string]map[string]string, *model.State) {
	cat := &model.SourceCatalog{SchemaVersion: 1, Entries: []model.SourceEntry{
		{Key: "a", Text: "A new"},
		{Key: "b", Text: "B new"},
		{Key: "c", Text: "C"},
		{Key: "d", Text: "D"},
		{Key: "e", Text: "E"},
		{Key: "f", Text: "F new"},
		{Key: "g", Text: "G"},
		{Key: "h", Text: "H new"},
	}}
	targets := map[string]map[string]string{
		"en": {
			"b": "B old",
			"c": "C",
			"d": "D human",
			"e": "E human first run",
			"f": "F approved",
			"g": "G locked",
			"h": "H edited",
			"x": "deleted target",
		},
	}
	st := model.NewState()
	st.Locale("en").Entries["b"] = &model.EntryState{SourceHash: hashx.Text("B old"), TargetHash: hashx.Text("B old")}
	st.Locale("en").Entries["c"] = &model.EntryState{SourceHash: hashx.Text("C"), TargetHash: hashx.Text("C")}
	st.Locale("en").Entries["d"] = &model.EntryState{SourceHash: hashx.Text("D"), TargetHash: hashx.Text("D human"), Manual: true}
	st.Locale("en").Entries["f"] = &model.EntryState{SourceHash: hashx.Text("F old"), TargetHash: hashx.Text("F approved"), Manual: true, Approved: true}
	st.Locale("en").Entries["g"] = &model.EntryState{SourceHash: hashx.Text("G"), TargetHash: hashx.Text("G locked"), Locked: true}
	st.Locale("en").Entries["h"] = &model.EntryState{SourceHash: hashx.Text("H old"), TargetHash: hashx.Text("H generated")}
	return cat, targets, st
}

func decisionFor(t *testing.T, result Result, key string) Decision {
	t.Helper()
	for _, d := range result.Decisions {
		if d.Key == key && d.Locale == "en" {
			return d
		}
	}
	t.Fatalf("no decision for %s", key)
	return Decision{}
}

func TestBuildClassifiesAllActions(t *testing.T) {
	cat, targets, st := fixture()
	result := Build(cat, targets, st, []string{"en"})
	cases := map[string]Action{
		"a": ActionNew,
		"b": ActionChanged,
		"c": ActionUnchanged,
		"d": ActionUnchanged,
		"e": ActionUnchanged,
		"f": ActionNeedsReview,
		"g": ActionUnchanged,
		"h": ActionNeedsReview,
		"x": ActionDeleted,
	}
	for key, want := range cases {
		if got := decisionFor(t, result, key).Action; got != want {
			t.Errorf("%s action = %s, want %s", key, got, want)
		}
	}
	if !decisionFor(t, result, "d").Manual {
		t.Error("manual edit not detected")
	}
	if !decisionFor(t, result, "e").Manual {
		t.Error("first-run human target not marked manual")
	}
	if !decisionFor(t, result, "h").Manual {
		t.Error("edited target with changed source not marked manual")
	}
	if !decisionFor(t, result, "f").Approved || !decisionFor(t, result, "g").Locked {
		t.Error("approval/lock flags missing from plan")
	}
	if result.Counts[ActionNew] != 1 || result.Counts[ActionChanged] != 1 || result.Counts[ActionNeedsReview] != 2 ||
		result.Counts[ActionDeleted] != 1 || result.Counts[ActionUnchanged] != 4 {
		t.Fatalf("unexpected counts: %#v", result.Counts)
	}
	if result.APIRequests != 2 {
		t.Fatalf("APIRequests = %d, want 2", result.APIRequests)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	cat, targets, st := fixture()
	a := Build(cat, targets, st, []string{"en"})
	b := Build(cat, targets, st, []string{"en"})
	if !reflect.DeepEqual(a, b) {
		t.Fatal("plan output is not deterministic")
	}
	for i := 1; i < len(a.Decisions); i++ {
		prev, cur := a.Decisions[i-1], a.Decisions[i]
		if prev.Locale > cur.Locale || prev.Locale == cur.Locale && prev.Key > cur.Key {
			t.Fatalf("decisions not sorted: %#v %#v", prev, cur)
		}
	}
}

func TestNewReportShape(t *testing.T) {
	cat, targets, st := fixture()
	result := Build(cat, targets, st, []string{"en"})
	report := NewReport("ja", []string{"en"}, result)
	if report.Kind != "plan" || report.SchemaVersion != 1 || report.Counts["new"] != 1 || report.APIRequests != 2 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if len(report.Actions) != len(result.Decisions) {
		t.Fatalf("actions = %d, decisions = %d", len(report.Actions), len(result.Decisions))
	}
}
