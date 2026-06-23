// Package shortener converts a monotonically increasing numeric ID into a
// fixed-length, non-enumerable 6-character base62 short code, and back.
//
// The mapping is a bijection on the domain [0, 62^6): an ID is first passed
// through a Feistel network (a keyed pseudo-random permutation) so that
// adjacent IDs produce wildly different codes (non-enumerable), then rendered
// as exactly 6 base62 characters. Because the transform is a bijection there
// are no collisions, and decoding is the exact inverse — no reverse lookup
// table is needed. The package is pure (no I/O) so it is trivially testable.
package shortener

import (
	"errors"
	"fmt"
)

const (
	// base62Alphabet defines the ordering used to render codes. Any fixed
	// ordering works because the Feistel layer already scrambles the value.
	base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	// CodeLength is the fixed number of characters in every short code.
	CodeLength = 6

	// base is the radix (len(base62Alphabet)).
	base = 62

	// halfBits is the width of each Feistel half. 2*halfBits = 36 bits gives a
	// domain of 2^36, the smallest power-of-two domain that contains MaxID.
	halfBits = 18
	halfMask = (1 << halfBits) - 1

	// rounds is the number of Feistel rounds. Four is the minimum for a Feistel
	// network to be a secure-ish PRP; we use more for better diffusion.
	rounds = 6
)

// MaxID is the exclusive upper bound of encodable IDs: 62^6.
const MaxID uint64 = 62 * 62 * 62 * 62 * 62 * 62 // 56,800,235,584

// feistelDomain is 2^(2*halfBits), the size of the Feistel permutation domain.
const feistelDomain uint64 = 1 << (2 * halfBits) // 2^36 = 68,719,476,736

// ErrIDOutOfRange is returned by Encode when id >= MaxID.
var ErrIDOutOfRange = errors.New("shortener: id out of range")

// ErrInvalidCode is returned by Decode when the code is malformed (wrong
// length or contains characters outside the base62 alphabet).
var ErrInvalidCode = errors.New("shortener: invalid code")

// decodeTable maps a byte to its base62 value, or -1 if not in the alphabet.
var decodeTable [256]int

func init() {
	for i := range decodeTable {
		decodeTable[i] = -1
	}
	for i := 0; i < len(base62Alphabet); i++ {
		decodeTable[base62Alphabet[i]] = i
	}
}

// Shortener encodes/decodes IDs using a keyed Feistel permutation. It is
// safe for concurrent use: all fields are read-only after construction.
type Shortener struct {
	roundKeys [rounds]uint64
}

// New returns a Shortener whose permutation is parameterized by key. The same
// key must be used to decode a code that was produced with it; a different key
// yields a different (but still valid) bijection.
func New(key uint64) *Shortener {
	s := &Shortener{}
	// Derive distinct per-round subkeys from key via a SplitMix64-style mix so
	// that each round uses well-separated key material.
	k := key
	for i := 0; i < rounds; i++ {
		k += 0x9E3779B97F4A7C15
		z := k
		z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
		z = (z ^ (z >> 27)) * 0x94D049BB133111EB
		z = z ^ (z >> 31)
		s.roundKeys[i] = z
	}
	return s
}

// Encode maps an ID in [0, MaxID) to a 6-character base62 code.
func (s *Shortener) Encode(id uint64) (string, error) {
	if id >= MaxID {
		return "", fmt.Errorf("%w: %d >= %d", ErrIDOutOfRange, id, MaxID)
	}
	x := s.obfuscate(id)
	return toBase62(x), nil
}

// Decode maps a 6-character base62 code back to its original ID.
func (s *Shortener) Decode(code string) (uint64, error) {
	x, err := fromBase62(code)
	if err != nil {
		return 0, err
	}
	// A well-formed 6-char base62 string can encode up to 62^6-1 == MaxID-1,
	// so x is always < MaxID; no extra range check is needed here.
	return s.deobfuscate(x), nil
}

// obfuscate maps id (in [0, MaxID)) to a scrambled value in [0, MaxID) using
// Feistel encryption plus cycle-walking to stay within the non-power-of-two
// domain. The result is a bijection on [0, MaxID).
func (s *Shortener) obfuscate(id uint64) uint64 {
	x := id
	for {
		x = s.feistelEncrypt(x)
		if x < MaxID {
			return x
		}
	}
}

// deobfuscate is the exact inverse of obfuscate.
func (s *Shortener) deobfuscate(x uint64) uint64 {
	v := x
	for {
		v = s.feistelDecrypt(v)
		if v < MaxID {
			return v
		}
	}
}

// feistelEncrypt applies the Feistel network to a value in [0, feistelDomain).
func (s *Shortener) feistelEncrypt(v uint64) uint64 {
	l := uint32((v >> halfBits) & halfMask)
	r := uint32(v & halfMask)
	for i := 0; i < rounds; i++ {
		l, r = r, l^s.roundFunc(r, i)
	}
	return (uint64(l) << halfBits) | uint64(r)
}

// feistelDecrypt is the inverse of feistelEncrypt.
func (s *Shortener) feistelDecrypt(v uint64) uint64 {
	l := uint32((v >> halfBits) & halfMask)
	r := uint32(v & halfMask)
	for i := rounds - 1; i >= 0; i-- {
		l, r = r^s.roundFunc(l, i), l
	}
	return (uint64(l) << halfBits) | uint64(r)
}

// roundFunc is the Feistel round function: a keyed mix of the half-block,
// reduced to halfBits. It need not be invertible.
func (s *Shortener) roundFunc(half uint32, round int) uint32 {
	x := uint64(half) ^ s.roundKeys[round]
	x *= 0x9E3779B97F4A7C15
	x ^= x >> 29
	x *= 0xBF58476D1CE4E5B9
	x ^= x >> 32
	return uint32(x) & halfMask
}

// toBase62 renders x as exactly CodeLength characters, left-padded with the
// zero digit. x must be < MaxID (== base^CodeLength), guaranteed by callers.
func toBase62(x uint64) string {
	var buf [CodeLength]byte
	for i := CodeLength - 1; i >= 0; i-- {
		buf[i] = base62Alphabet[x%base]
		x /= base
	}
	return string(buf[:])
}

// fromBase62 parses a CodeLength-character base62 string into its value,
// rejecting wrong lengths and out-of-alphabet characters.
func fromBase62(code string) (uint64, error) {
	if len(code) != CodeLength {
		return 0, fmt.Errorf("%w: length %d != %d", ErrInvalidCode, len(code), CodeLength)
	}
	var x uint64
	for i := 0; i < len(code); i++ {
		d := decodeTable[code[i]]
		if d < 0 {
			return 0, fmt.Errorf("%w: bad character %q", ErrInvalidCode, code[i])
		}
		x = x*base + uint64(d)
	}
	return x, nil
}
