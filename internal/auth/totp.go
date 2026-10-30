package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func NewTOTPSecret() string {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b32.EncodeToString(b)
}

func TOTPURL(issuer, account, secret string) string {
	v := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return fmt.Sprintf("otpauth://totp/%s:%s?%s", url.PathEscape(issuer), url.PathEscape(account), v.Encode())
}

func totpAt(secret string, counter uint64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(secret))
	if err != nil {
		return "", err
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	h := hmac.New(sha1.New, key)
	h.Write(msg[:])
	sum := h.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1_000_000), nil
}

// TOTPCode renvoie le code valide à l'instant t (utile aux tests et aux clients).
func TOTPCode(secret string, t time.Time) (string, error) {
	return totpAt(secret, uint64(t.Unix()/30))
}

// ValidateTOTP accepte le code courant et ceux des fenêtres adjacentes (±30 s).
func ValidateTOTP(secret, code string, now time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return false
	}
	c := uint64(now.Unix() / 30)
	for _, d := range []int64{0, -1, 1} {
		want, err := totpAt(secret, uint64(int64(c)+d))
		if err == nil && hmac.Equal([]byte(want), []byte(code)) {
			return true
		}
	}
	return false
}

// NewRecoveryCodes renvoie n codes de secours lisibles (xxxxx-xxxxx).
func NewRecoveryCodes(n int) []string {
	out := make([]string, n)
	for i := range out {
		b := make([]byte, 7)
		if _, err := rand.Read(b); err != nil {
			panic(err)
		}
		s := strings.ToLower(b32.EncodeToString(b))[:10]
		out[i] = s[:5] + "-" + s[5:]
	}
	return out
}
