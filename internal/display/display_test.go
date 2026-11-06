package display

import "testing"

func TestParseReport(t *testing.T) {
	report, err := Parse([]byte(`{"schema_version":1,"checked":7,"issues":[{"key":"ui.start","locale":"en","node_path":"Main/Label","kind":"overflow","message":"clipped"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if report.Checked != 7 || len(report.Issues) != 1 || report.Issues[0].NodePath != "Main/Label" {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	cases := []string{
		`{"schema_version":2,"issues":[],"checked":0}`,
		`{"schema_version":1,"checked":-1,"issues":[]}`,
		`{"schema_version":1,"checked":0,"issues":[{"key":"","locale":"en","kind":"overflow"}]}`,
		`{"schema_version":1,"checked":0,"issues":[{"key":"k","locale":"en","kind":""}]}`,
		`{"schema_version":1,"checked":0,"issues":[],"extra":true}`,
		`{"schema_version":1,"checked":0,"issues":[]} trailing`,
	}
	for _, input := range cases {
		if _, err := Parse([]byte(input)); err == nil {
			t.Errorf("Parse(%s) unexpectedly succeeded", input)
		}
	}
}
