package savestate

import "testing"

func TestLongestCommonPrefix(t *testing.T) {
	if got := longestCommonPrefix([]string{"save/Core", "save/Input"}); got != "save/" {
		t.Fatalf("got %q", got)
	}
	if got := longestCommonPrefix([]string{"a", "b"}); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestOpenZipArchiveRejectsGarbage(t *testing.T) {
	if _, err := openZipArchive([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected error")
	}
}
