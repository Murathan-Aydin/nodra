package crypto

import "testing"

func TestRoundTrip(t *testing.T) {
	b, err := New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := b.Encrypt("secret")
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.Decrypt(enc)
	if err != nil || got != "secret" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := b.Decrypt(enc[:len(enc)-4] + "AAAA"); err == nil {
		t.Fatal("un texte altéré doit être rejeté")
	}
}
