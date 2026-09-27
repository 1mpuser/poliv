package secretbox

import (
	"strings"
	"testing"
)

func TestRoundtripAndHidesToken(t *testing.T) {
	box, err := Encrypt("y0_secret-token", "jwt-secret")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(box, "y0_secret") {
		t.Fatal("token leaked")
	}
	dec, err := Decrypt(box, "jwt-secret")
	if err != nil {
		t.Fatal(err)
	}
	if dec != "y0_secret-token" {
		t.Fatalf("dec %q", dec)
	}
}

func TestOtherSecretOrGarbageGivesError(t *testing.T) {
	box, _ := Encrypt("y0_secret-token", "jwt-secret")
	if _, err := Decrypt(box, "другой секрет"); err == nil {
		t.Fatal("wrong secret accepted")
	}
	if _, err := Decrypt("мусор", "jwt-secret"); err == nil {
		t.Fatal("garbage accepted")
	}
}
