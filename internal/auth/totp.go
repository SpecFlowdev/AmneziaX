package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP as authenticator apps actually implement it: RFC 6238 over HMAC-SHA1,
// six digits, a thirty second step. Those three are not configurable anywhere in
// the ecosystem worth supporting, so they are constants rather than settings
// that could be set to something no app can read.
const (
	totpDigits = 6
	totpPeriod = 30 * time.Second

	// A phone whose clock has drifted is the ordinary case, not an attack, so
	// one step either side is accepted. Wider than that starts handing an
	// attacker extra codes for free.
	totpSkew = 1
)

var totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewTOTPSecret returns a base32 secret in the form authenticator apps expect.
// Twenty bytes matches the HMAC-SHA1 block the algorithm keys with, so nothing
// is truncated or stretched.
func NewTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return totpEncoding.EncodeToString(raw), nil
}

// TOTPURI builds the otpauth:// URI that a QR code encodes. The issuer appears
// twice by convention — once in the label and once as a parameter — because
// different apps read one or the other.
func TOTPURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(totpDigits))
	q.Set("period", fmt.Sprint(int(totpPeriod.Seconds())))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// TOTPCode computes the code for one moment. Exported so the tests can prove the
// implementation against the RFC's published vectors rather than against itself.
func TOTPCode(secret string, at time.Time) (string, error) {
	key, err := totpEncoding.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", fmt.Errorf("invalid totp secret: %w", err)
	}
	counter := uint64(at.Unix()) / uint64(totpPeriod.Seconds())

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	// Dynamic truncation, RFC 4226 §5.3: the low nibble of the last byte picks
	// where in the digest the code is read from.
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	mod := uint32(1)
	for i := 0; i < totpDigits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", totpDigits, value%mod), nil
}

// CheckTOTP reports whether code is valid for secret right now. The comparison
// is constant-time: a code is a six digit secret, and an early return would leak
// how much of it was right.
func CheckTOTP(secret, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}
	now := time.Now()
	ok := false
	for skew := -totpSkew; skew <= totpSkew; skew++ {
		want, err := TOTPCode(secret, now.Add(time.Duration(skew)*totpPeriod))
		if err != nil {
			return false
		}
		// No early break: every candidate is compared so the loop takes the
		// same time whichever step matches.
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			ok = true
		}
	}
	return ok
}

// TOTPStep identifies the time step a code belongs to. Storing the last accepted
// step is what stops the same code being replayed inside its 30 second window —
// without it, a code shoulder-surfed or caught in a proxy log is reusable until
// it expires.
func TOTPStep(at time.Time) int64 {
	return at.Unix() / int64(totpPeriod.Seconds())
}

// Recovery codes are the way back in when the phone is gone. They are shown
// once, stored hashed like any other credential, and each works a single time.
const recoveryCodeCount = 10

// NewRecoveryCodes returns the codes to show the operator and the digests to
// store. Grouping into two blocks of four makes them transcribable without
// making them meaningfully shorter.
func NewRecoveryCodes() (plain []string, digests []string, err error) {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no O/0, I/1
	for i := 0; i < recoveryCodeCount; i++ {
		raw := make([]byte, 8)
		if _, err = rand.Read(raw); err != nil {
			return nil, nil, err
		}
		var b strings.Builder
		for j, v := range raw {
			if j == 4 {
				b.WriteByte('-')
			}
			b.WriteByte(alphabet[int(v)%len(alphabet)])
		}
		code := b.String()
		plain = append(plain, code)
		digests = append(digests, HashToken(normaliseRecoveryCode(code)))
	}
	return plain, digests, nil
}

// MatchRecoveryCode returns the index of the digest the code belongs to, or -1.
// Every digest is compared so a wrong code costs the same as a right one.
func MatchRecoveryCode(digests []string, code string) int {
	want := HashToken(normaliseRecoveryCode(code))
	found := -1
	for i, d := range digests {
		if subtle.ConstantTimeCompare([]byte(d), []byte(want)) == 1 {
			found = i
		}
	}
	return found
}

// normaliseRecoveryCode forgives the ways a person retypes one: lower case, and
// with or without the separator.
func normaliseRecoveryCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	return strings.ReplaceAll(code, "-", "")
}
