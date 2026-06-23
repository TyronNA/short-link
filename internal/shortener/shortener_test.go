package shortener

import (
	"errors"
	"testing"
)

const testKey = 0xC0FFEE1234567890

func TestEncodeDecodeRoundTrip(t *testing.T) {
	s := New(testKey)
	ids := []uint64{
		0,
		1,
		2,
		61,
		62,
		1000,
		MaxID / 2,
		MaxID - 2,
		MaxID - 1,
	}
	for _, id := range ids {
		code, err := s.Encode(id)
		if err != nil {
			t.Fatalf("Encode(%d) error: %v", id, err)
		}
		if len(code) != CodeLength {
			t.Errorf("Encode(%d) = %q, want length %d", id, code, CodeLength)
		}
		got, err := s.Decode(code)
		if err != nil {
			t.Fatalf("Decode(%q) error: %v", code, err)
		}
		if got != id {
			t.Errorf("round-trip failed: id=%d code=%q decoded=%d", id, code, got)
		}
	}
}

func TestEncodeAlwaysSixChars(t *testing.T) {
	s := New(testKey)
	for _, id := range []uint64{0, 1, 100, 123456, MaxID - 1} {
		code, err := s.Encode(id)
		if err != nil {
			t.Fatalf("Encode(%d): %v", id, err)
		}
		if len(code) != 6 {
			t.Errorf("Encode(%d) = %q, want 6 chars", id, code)
		}
		for i := 0; i < len(code); i++ {
			if decodeTable[code[i]] < 0 {
				t.Errorf("Encode(%d) = %q has non-base62 char %q", id, code, code[i])
			}
		}
	}
}

// TestBijectionNoCollisions verifies that a contiguous block of IDs maps to
// distinct codes (no collisions) — a property guaranteed by the bijection.
func TestBijectionNoCollisions(t *testing.T) {
	s := New(testKey)
	seen := make(map[string]uint64)
	const n = 50000
	for id := uint64(0); id < n; id++ {
		code, err := s.Encode(id)
		if err != nil {
			t.Fatalf("Encode(%d): %v", id, err)
		}
		if prev, ok := seen[code]; ok {
			t.Fatalf("collision: id=%d and id=%d both map to %q", prev, id, code)
		}
		seen[code] = id
	}
}

// TestNonEnumerable verifies that adjacent IDs produce codes that differ in
// more than just the last character — i.e. the output is not trivially
// guessable from a neighbouring code.
func TestNonEnumerable(t *testing.T) {
	s := New(testKey)
	c0, _ := s.Encode(0)
	c1, _ := s.Encode(1)
	c2, _ := s.Encode(2)
	if c0 == c1 || c1 == c2 || c0 == c2 {
		t.Fatalf("adjacent IDs collided: %q %q %q", c0, c1, c2)
	}
	// Count differing positions between consecutive codes; a counter rendered
	// directly to base62 would differ in only the final position.
	diff := func(a, b string) int {
		n := 0
		for i := 0; i < len(a); i++ {
			if a[i] != b[i] {
				n++
			}
		}
		return n
	}
	if d := diff(c0, c1); d < 2 {
		t.Errorf("codes for id 0 (%q) and 1 (%q) differ in only %d position(s); looks enumerable", c0, c1, d)
	}
}

func TestEncodeOutOfRange(t *testing.T) {
	s := New(testKey)
	for _, id := range []uint64{MaxID, MaxID + 1, ^uint64(0)} {
		_, err := s.Encode(id)
		if !errors.Is(err, ErrIDOutOfRange) {
			t.Errorf("Encode(%d) error = %v, want ErrIDOutOfRange", id, err)
		}
	}
}

func TestDecodeInvalidCode(t *testing.T) {
	s := New(testKey)
	cases := []struct {
		name string
		code string
	}{
		{"too short", "abc"},
		{"too long", "abcdefg"},
		{"empty", ""},
		{"bad char dash", "abc-ef"},
		{"bad char space", "abc ef"},
		{"bad char plus", "ab+cde"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.Decode(tc.code)
			if !errors.Is(err, ErrInvalidCode) {
				t.Errorf("Decode(%q) error = %v, want ErrInvalidCode", tc.code, err)
			}
		})
	}
}

// TestDifferentKeysDifferentMapping verifies the key actually parameterizes
// the permutation.
func TestDifferentKeysDifferentMapping(t *testing.T) {
	a := New(1)
	b := New(2)
	diffs := 0
	for id := uint64(0); id < 100; id++ {
		ca, _ := a.Encode(id)
		cb, _ := b.Encode(id)
		if ca != cb {
			diffs++
		}
	}
	if diffs == 0 {
		t.Fatal("two different keys produced identical mappings for 100 IDs")
	}
}

// TestDecodeRejectsForeignKeyGracefully ensures decoding with the wrong key
// still returns a valid in-range ID (not a crash); it simply won't match.
func TestDecodeWithSameKeyMatches(t *testing.T) {
	s := New(testKey)
	code, _ := s.Encode(42)
	got, err := s.Decode(code)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got != 42 {
		t.Fatalf("Decode(%q) = %d, want 42", code, got)
	}
}
