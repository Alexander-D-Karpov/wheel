// Package idgen produces the identifiers and randomness the service needs.
// Everything comes from crypto/rand so that spin outcomes cannot be predicted
// from previous results.
package idgen

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
)

// slugAlphabet omits look-alike characters so share links survive being read
// out loud or retyped.
const slugAlphabet = "23456789abcdefghijkmnpqrstuvwxyz"

// UUID returns a random RFC 4122 version 4 identifier.
func UUID() string {
	var b [16]byte
	mustRead(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}

// Token returns a 64 character hex string used as the session cookie value.
func Token() string {
	var b [32]byte
	mustRead(b[:])
	return hex.EncodeToString(b[:])
}

// Slug returns an n character share-link identifier.
func Slug(n int) string {
	if n < 1 {
		n = 1
	}
	buf := make([]byte, n)
	out := make([]byte, n)
	for i := 0; i < n; {
		mustRead(buf)
		for _, v := range buf {
			// Reject the tail of the byte range so every symbol stays equally likely.
			if int(v) >= 256-(256%len(slugAlphabet)) {
				continue
			}
			out[i] = slugAlphabet[int(v)%len(slugAlphabet)]
			i++
			if i == n {
				break
			}
		}
	}
	return string(out)
}

// Float returns a uniform float64 in [0, 1).
func Float() float64 {
	var b [8]byte
	mustRead(b[:])
	v := binary.BigEndian.Uint64(b[:]) >> 11
	f := float64(v) / float64(uint64(1)<<53)
	if f >= 1 {
		f = math.Nextafter(1, 0)
	}
	return f
}

// Int returns a uniform integer in [0, n). It returns 0 for n <= 0.
func Int(n int) int {
	if n <= 0 {
		return 0
	}
	v := int(Float() * float64(n))
	if v >= n {
		v = n - 1
	}
	return v
}

func mustRead(b []byte) {
	if _, err := rand.Read(b); err != nil {
		// crypto/rand never fails on the platforms this runs on; if it does,
		// continuing would mean handing out predictable spin results.
		panic("idgen: crypto/rand unavailable: " + err.Error())
	}
}
