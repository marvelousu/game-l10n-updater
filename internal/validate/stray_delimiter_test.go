package validate

import "testing"

func TestStrayDelimiterDoesNotHideTag(t *testing.T) {
	if len(Mismatches("HP < 50 <b>warning</b>", "HP below 50 <b>warning</b>")) != 0 {
		t.Fatal("stray less-than hid real opening tag")
	}
	if len(Mismatches("<b>warning</b>", "HP < 50 warning</b>")) == 0 {
		t.Fatal("missing opening tag falsely passed")
	}
	if len(Mismatches("[ < [b]x[/b]", "[b]x[/b]")) != 0 {
		t.Fatal("stray bracket hid a real BBCode tag")
	}
}
