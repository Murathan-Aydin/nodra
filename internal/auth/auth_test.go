package auth

import (
	"testing"
	"time"
)

func TestPassword(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := VerifyPassword("correct horse", h); !ok {
		t.Fatal("le bon mot de passe doit être accepté")
	}
	if ok, _ := VerifyPassword("wrong horse", h); ok {
		t.Fatal("un mauvais mot de passe doit être refusé")
	}
	if _, err := HashPassword("court"); err != ErrWeakPassword {
		t.Fatal("un mot de passe trop court doit être refusé")
	}
}

func TestTOTPVector(t *testing.T) {
	// Vecteur RFC 6238 (SHA1, secret ASCII "12345678901234567890", T=59 -> 94287082, 6 derniers chiffres).
	secret := b32.EncodeToString([]byte("12345678901234567890"))
	if !ValidateTOTP(secret, "287082", time.Unix(59, 0)) {
		t.Fatal("le vecteur RFC 6238 doit être validé")
	}
	if ValidateTOTP(secret, "287082", time.Unix(59+120, 0)) {
		t.Fatal("un code expiré doit être refusé")
	}
}

func TestTokens(t *testing.T) {
	a, b := NewToken(PrefixAPIToken), NewToken(PrefixAPIToken)
	if a == b || HashToken(a) == HashToken(b) || len(HashToken(a)) != 64 {
		t.Fatal("jetons non uniques ou hash invalide")
	}
	if len(NewRecoveryCodes(8)) != 8 {
		t.Fatal("8 codes attendus")
	}
}
