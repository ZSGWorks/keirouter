package consolelog

import "testing"

func TestSetMinLevelFiltersEntries(t *testing.T) {
	b := New()
	b.SetMinLevel("warn")

	b.Log("DEBUG", "debug noise", "")
	b.Log("INFO", "info noise", "")
	b.Log("LOG", "plain noise", "")
	b.Log("WARN", "kept warn", "")
	b.Log("ERROR", "kept error", "")

	got := b.Entries()
	if len(got) != 2 {
		t.Fatalf("entries = %v; want only WARN+ERROR", got)
	}
	if got[0].Level != "WARN" || got[1].Level != "ERROR" {
		t.Fatalf("levels = %q, %q", got[0].Level, got[1].Level)
	}
}

func TestSetMinLevelDebugKeepsEverything(t *testing.T) {
	b := New()
	b.SetMinLevel("debug")
	b.Log("DEBUG", "d", "")
	b.Log("LOG", "l", "")
	if len(b.Entries()) != 2 {
		t.Fatalf("entries = %d; want 2", len(b.Entries()))
	}
}

func TestMinLevelRankForUnknownLevelKeepsAll(t *testing.T) {
	if got := minLevelRankFor(""); got != levelRank["LOG"] {
		t.Fatalf("rank = %d; want default LOG rank", got)
	}
	if got := minLevelRankFor("error"); got != levelRank["ERROR"] {
		t.Fatalf("rank = %d; want ERROR rank", got)
	}
}
