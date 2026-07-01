package bot

import (
	"encoding/base64"
	"testing"
)

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

func TestCalculatePHashSupportsWebP(t *testing.T) {
	webpData, err := base64.StdEncoding.DecodeString(
		"UklGRkgCAABXRUJQVlA4IDwCAABwDACdASoqACwAPpE6mEiloyIhLjgLMLASCWYAsR72f7yBLD2LAs/vazEi2+eEOPxR6D+VRAijRbfM3QDMq3h4pTXabcKyb3cD/zWbAvFiDsJBS4o7pS7xrl1aaDOmlkSFKNfA70FCciW6HkAA/aoPCI+d2+XLd/gP118vv3WbMMkq4h5lBxaqO6GEHVaq84R/ce1FNfFhiNaVNFkTp4GF1RFpwv4wpiSXO+c5+MqjGr8bCfidokhnoDznEhiyXvE43ALQ6c5qbrQzv60K3BROjZ227rCzvQoRopKrUw1X97ehd9i7Y2Eqt1Fk3nbMgMafWIBn81dwBiN7VvpjS0TJuRt0qxyh5IMM3AGXeqGW6Kufn/WDRG9dBBHd1AfV/2fkT+S756cyKAThhZf+bERDdQrmhffhe8ejLVm0e2EDXKJR7C+QMFNHz9WMPP2ZhpMO87korvEy/dpnPQMmB7NNcTBLagsVbsP5iM9iP0Qj6nzpFU2Couqz0adR+UaKSXjh64VvtzK3evQtQ4nY7TPGU6VqtTQlf3K7ONLUSct27PSWaEAX6lG5aDBaB0Z4t1tbvfGqyQA3ZbjwICfXJ+Eq/M905DUlQ9Cvse77GPBVxjEEFSdCUPXuR1GcWfvJdS6LyAuQLQC8IdEFX1rawtOACWYAZc/a8J4Bs1OcNsnwRAQgMc8MUFt03tEXfVpbFgzd+qZwgkdYE919YDyPKuk/DH0920x4YfsfsT4NfCM3IKQm2AUe6TevLZgAAA==",
	)
	if err != nil {
		t.Fatalf("decode webp fixture: %v", err)
	}

	if _, err := calculatePHash(webpData); err != nil {
		t.Fatalf("calculatePHash should support webp: %v", err)
	}
}
