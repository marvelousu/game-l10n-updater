package state

import (
	"strings"
	"testing"

	"github.com/marvelousu/game-l10n-updater/internal/model"
)

func TestParseAndMarshal(t *testing.T) {
	data := []byte(`{"schema_version":1,"files":{"en":{"entries":{"ui.start":{"source_hash":"","target_hash":"","manual":true,"approved":true}}}}}`)
	st, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	entry := st.Entry("en", "ui.start")
	if entry == nil || !entry.Manual || !entry.Approved {
		t.Fatalf("unexpected entry: %#v", entry)
	}
	out, err := Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"manual": true`) || !strings.HasSuffix(string(out), "\n") {
		t.Fatalf("unexpected state JSON:\n%s", out)
	}
}

func TestParseRejectsSchemaAndBadLocale(t *testing.T) {
	cases := []string{
		`{"schema_version":2,"files":{}}`,
		`{"schema_version":1,"files":{"../en":{"entries":{}}}}`,
		`{"schema_version":1,"files":{"en":{"entries":{"a":{"source_hash":"not-a-hash"}}}}}`,
		`{"schema_version":1,"files":{},"extra":1}`,
	}
	for _, input := range cases {
		if _, err := Parse([]byte(input)); err == nil {
			t.Errorf("Parse(%s) unexpectedly succeeded", input)
		}
	}
}

func TestCloneIsDeep(t *testing.T) {
	st := model.NewState()
	st.Locale("en").Entries["a"] = &model.EntryState{Manual: true}
	cp := Clone(st)
	cp.Locale("en").Entries["a"].Manual = false
	if !st.Entry("en", "a").Manual {
		t.Fatal("Clone shared entry state")
	}
}
