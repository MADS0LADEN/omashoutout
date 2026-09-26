package audio

import "testing"

func TestModuleIDFromShortList(t *testing.T) {
	text := "1\tmodule-null-sink\tsink_name=other\n" +
		"536870916\tmodule-null-sink\tsink_name=shoutout shoutout.owner=shoutout\n" +
		"42\tmodule-null-sink\tsink_name=omashoutout omashoutout.owner=omashoutout\n"
	id, ok := moduleIDFromShortList(text, legacyOwnerMarker)
	if !ok || id != "536870916" {
		t.Fatalf("legacy id = %q ok=%v", id, ok)
	}
	id, ok = moduleIDFromShortList(text, ownerMarker)
	if !ok || id != "42" {
		t.Fatalf("current id = %q ok=%v", id, ok)
	}
	if _, ok = moduleIDFromShortList(text, "missing"); ok {
		t.Fatal("expected no match")
	}
}
