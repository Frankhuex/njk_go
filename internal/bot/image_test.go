package bot

import "testing"

func TestNormalizeStoredHashSupportsGoImageHashPrefix(t *testing.T) {
	if got := normalizeStoredHash("p:0123456789abcdef"); got != "0123456789abcdef" {
		t.Fatalf("unexpected normalized hash: %s", got)
	}
}

func TestDecodeHashSupportsLegacyAndCurrentFormats(t *testing.T) {
	current, err := decodeHash("0123456789abcdef")
	if err != nil {
		t.Fatalf("decode current hash failed: %v", err)
	}
	legacy, err := decodeHash("p:0123456789abcdef")
	if err != nil {
		t.Fatalf("decode legacy hash failed: %v", err)
	}
	if string(current) != string(legacy) {
		t.Fatal("expected current and legacy hash formats to decode identically")
	}
}
