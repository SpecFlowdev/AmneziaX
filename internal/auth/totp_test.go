package auth

import (
	"strings"
	"testing"
	"time"
)

// The RFC 6238 appendix B vectors use the ASCII seed "12345678901234567890".
// Testing against them is the only way to know the implementation matches what
// an authenticator app will compute, rather than merely matching itself.
func TestTOTPCodeMatchesRFC6238Vectors(t *testing.T) {
	secret := totpEncoding.EncodeToString([]byte("12345678901234567890"))

	// The RFC prints eight digits; these are the low six, which is what a
	// six-digit authenticator shows for the same moment.
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	}
	for _, c := range cases {
		got, err := TOTPCode(secret, time.Unix(c.unix, 0))
		if err != nil {
			t.Fatalf("unix %d: %v", c.unix, err)
		}
		if got != c.want {
			t.Errorf("unix %d: got %s, want %s", c.unix, got, c.want)
		}
	}
}

func TestCheckTOTPAcceptsCurrentAndAdjacentSteps(t *testing.T) {
	secret, err := NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()

	for _, offset := range []time.Duration{-totpPeriod, 0, totpPeriod} {
		code, err := TOTPCode(secret, now.Add(offset))
		if err != nil {
			t.Fatal(err)
		}
		if !CheckTOTP(secret, code) {
			t.Errorf("code for offset %v was rejected", offset)
		}
	}

	// Two steps out is outside the window a drifting clock explains.
	far, err := TOTPCode(secret, now.Add(3*totpPeriod))
	if err != nil {
		t.Fatal(err)
	}
	if CheckTOTP(secret, far) {
		t.Error("a code three steps away was accepted")
	}
}

func TestCheckTOTPRejectsMalformed(t *testing.T) {
	secret, err := NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"", "12345", "1234567", "abcdef", "      "} {
		if CheckTOTP(secret, code) {
			t.Errorf("accepted malformed code %q", code)
		}
	}
}

func TestNewTOTPSecretDecodesAndDiffers(t *testing.T) {
	a, err := NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two secrets came out identical")
	}
	// An app will reject a secret it cannot base32-decode, so the encoding
	// matters as much as the entropy.
	if _, err := totpEncoding.DecodeString(a); err != nil {
		t.Fatalf("secret is not valid base32: %v", err)
	}
}

func TestTOTPURICarriesWhatAppsRead(t *testing.T) {
	uri := TOTPURI("AmneziaX", "admin", "ABCDEFGHIJKLMNOP")
	for _, want := range []string{
		"otpauth://totp/",
		"secret=ABCDEFGHIJKLMNOP",
		"issuer=AmneziaX",
		"digits=6",
		"period=30",
		"algorithm=SHA1",
	} {
		if !strings.Contains(uri, want) {
			t.Errorf("uri %q is missing %q", uri, want)
		}
	}
}

func TestRecoveryCodesAreSingleUseAndForgiving(t *testing.T) {
	plain, digests, err := NewRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(plain) != recoveryCodeCount || len(digests) != recoveryCodeCount {
		t.Fatalf("got %d/%d codes, want %d", len(plain), len(digests), recoveryCodeCount)
	}

	seen := map[string]bool{}
	for _, c := range plain {
		if seen[c] {
			t.Fatalf("duplicate recovery code %q", c)
		}
		seen[c] = true
	}

	// Whichever way it is retyped, it has to land on the same digest.
	code := plain[3]
	for _, variant := range []string{
		code,
		strings.ToLower(code),
		strings.ReplaceAll(code, "-", ""),
		"  " + strings.ToLower(strings.ReplaceAll(code, "-", "")) + "  ",
	} {
		if got := MatchRecoveryCode(digests, variant); got != 3 {
			t.Errorf("variant %q matched index %d, want 3", variant, got)
		}
	}

	if got := MatchRecoveryCode(digests, "ZZZZ-ZZZZ"); got != -1 {
		t.Errorf("an unknown code matched index %d, want -1", got)
	}
}
