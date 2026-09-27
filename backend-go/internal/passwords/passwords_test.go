package passwords

import (
	"strings"
	"testing"
)

func TestHashRoundtrip(t *testing.T) {
	h, err := HashPassword("секрет-123")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "scrypt$") {
		t.Fatalf("prefix %q", h)
	}
	if !VerifyPassword("секрет-123", h) {
		t.Fatal("verify true failed")
	}
	if VerifyPassword("секрет-124", h) {
		t.Fatal("verify false failed")
	}
}

func TestSamePasswordDifferentSalt(t *testing.T) {
	a, _ := HashPassword("x-123456789")
	b, _ := HashPassword("x-123456789")
	if a == b {
		t.Fatal("same hash")
	}
}

func TestPlaceholderHashNeverMatches(t *testing.T) {
	if VerifyPassword("", "!") {
		t.Fatal("empty matched")
	}
	if VerifyPassword("!", "!") {
		t.Fatal("bang matched")
	}
}

func TestGeneratedPassword(t *testing.T) {
	p, err := GeneratePassword(16)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 16 {
		t.Fatalf("len %d", len(p))
	}
	for _, c := range p {
		if !containsRune(Alphabet, c) {
			t.Fatalf("bad char %q", c)
		}
		if c == '0' || c == 'O' || c == '1' || c == 'l' || c == 'I' {
			t.Fatalf("ambiguous char %q", c)
		}
	}
	q, _ := GeneratePassword(16)
	if p == q {
		t.Fatal("deterministic")
	}
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}
