package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"strings"
)

// NewToken génère un jeton aléatoire de 32 octets. Seul son hash (HashToken) est stocké.
func NewToken(prefix string) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

func HashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

// Préfixes : facilitent la détection de fuites (secret scanning).
const (
	PrefixSession  = "nds_"
	PrefixAPIToken = "ndt_"
	PrefixReset    = "ndr_"
	PrefixInvite   = "ndi_"
	PrefixVerify   = "ndv_"
	PrefixOAuth    = "ndo_"
)
