package main

import (
	"testing"
)

func TestGlyphsAreFiveBySeven(t *testing.T) {
	for r, g := range font {
		for i, row := range g {
			if len(row) != 5 {
				t.Errorf("glyph %q row %d has width %d", r, i, len(row))
			}
		}
	}
}

func TestWrapText(t *testing.T) {
	got := wrapText("  Merci   Emma, c'était génial ❤️ ", 11)
	want := []string{"MERCI EMMA,", "C'ETAIT", "GENIAL ♥"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %q, want %q", len(got), got, want)
	}
	for i := range want {
		if string(got[i]) != want[i] {
			t.Errorf("line %d: got %q, want %q", i, string(got[i]), want[i])
		}
	}
	for _, l := range got {
		for _, r := range l {
			if _, ok := font[r]; !ok && r != ' ' {
				t.Errorf("no glyph for %q", r)
			}
		}
	}
	if got := wrapText("   ", 10); got != nil {
		t.Errorf("blank message should give no lines, got %q", got)
	}
}

func TestParseArgs(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"-c", "/usr/local/bin/fireworks"}, "default"},
		{[]string{"-message", "Salut"}, "Salut"},
		{[]string{"--message=Bravo !"}, "Bravo !"},
		{[]string{"-m", ""}, ""},
	}
	t.Setenv("FIREWORKS_MESSAGE", "default")
	for _, c := range cases {
		message = "unset"
		parseArgs(c.args)
		if message != c.want {
			t.Errorf("parseArgs(%q) = %q, want %q", c.args, message, c.want)
		}
	}
}
