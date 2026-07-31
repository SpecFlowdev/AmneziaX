// Package auth issues and validates panel credentials.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/SpecFlowdev/AmneziaX/internal/domain"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidToken = errors.New("invalid token")

type Claims struct {
	AdminID  string           `json:"sub"`
	Username string           `json:"username"`
	Role     domain.AdminRole `json:"role"`
	jwt.RegisteredClaims
}

type Issuer struct {
	secret []byte
	ttl    time.Duration
}

func NewIssuer(secret string, ttl time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), ttl: ttl}
}

func (i *Issuer) TTL() time.Duration { return i.ttl }

func (i *Issuer) Issue(a *domain.Admin) (string, time.Time, error) {
	expires := time.Now().Add(i.ttl)
	claims := Claims{
		AdminID:  a.UUID,
		Username: a.Username,
		Role:     a.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   a.UUID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expires),
			Issuer:    "amneziax",
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expires, nil
}

func (i *Issuer) Parse(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return i.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return nil, ErrInvalidToken
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// HashPassword uses bcrypt, which keeps verification slow enough to make an
// offline attack on a leaked dump expensive.
func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(h), err
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// NewNodeToken returns a fresh enrolment secret together with the digest stored
// in the database and a preview safe to show in the UI.
func NewNodeToken() (token, digest, preview string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	digest = HashToken(token)
	if len(token) > 6 {
		preview = "…" + token[len(token)-6:]
	}
	return
}

// HashToken digests a node token. Node tokens are high-entropy random strings,
// so a plain SHA-256 is enough and keeps agent handshakes cheap.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func TokenMatches(digest, token string) bool {
	return subtle.ConstantTimeCompare([]byte(digest), []byte(HashToken(token))) == 1
}

// RandomSecret returns a URL-safe random string of the given byte length.
func RandomSecret(n int) string {
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
