package deploy

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/crypto"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// sslSpec : prise en charge de TLS par type de base (options reprises de Coolify).
type sslSpec struct {
	Dir   string   // dossier des certificats dans le conteneur
	Owner string   // propriétaire des fichiers attendu par le moteur
	Image bool     // propriétaire résolu dans l'image (docker compose run --user root chown) plutôt que sur l'hôte
	Modes []string // modes proposés (premier : défaut) ; vide si le mode est un réglage client seulement
}

var SSLTypes = map[string]sslSpec{
	"postgresql": {"/var/lib/postgresql/certs", "postgres:postgres", true, []string{"require", "verify-ca", "verify-full", "prefer", "allow"}},
	"mysql":      {"/etc/mysql/certs", "mysql:mysql", true, nil},
	"mariadb":    {"/etc/mysql/certs", "mysql:mysql", true, nil},
	"mongodb":    {"/etc/mongo/certs", "mongodb:mongodb", true, []string{"requireTLS", "preferTLS", "allowTLS"}},
	"redis":      {"/etc/redis/certs", "999:999", false, nil},
	"keydb":      {"/etc/keydb/certs", "999:999", false, nil},
}

// ValidSSLMode : mode accepté pour le type ("" = mode par défaut).
func ValidSSLMode(typ, mode string) bool {
	spec, ok := SSLTypes[typ]
	return ok && (mode == "" || slices.Contains(spec.Modes, mode))
}

// SSLMode : mode effectif (défaut du type si vide).
func SSLMode(db *store.Database) string {
	if m := db.Settings.SSLMode; m != "" {
		return m
	}
	if spec := SSLTypes[db.Type]; len(spec.Modes) > 0 {
		return spec.Modes[0]
	}
	return ""
}

// sslEnabled : TLS demandé et pris en charge par le type.
func sslEnabled(db *store.Database) bool {
	_, ok := SSLTypes[db.Type]
	return ok && db.Settings.EnableSSL
}

func pemBlock(typ string, der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}))
}

func serial() *big.Int {
	n, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	return n
}

// GenerateCA crée une autorité de certification (RSA 2048, 10 ans) ; renvoie certificat et clé en PEM.
func GenerateCA(name string) (certPEM, keyPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	tpl := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: name, Organization: []string{"Nodra"}},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(10, 0, 0), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return "", "", err
	}
	return pemBlock("CERTIFICATE", der), pemBlock("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key)), nil
}

// IssueCert signe un certificat serveur (1 an) pour les noms et adresses donnés.
func IssueCert(caCertPEM, caKeyPEM, cn string, names []string) (certPEM, keyPEM string, err error) {
	cb, _ := pem.Decode([]byte(caCertPEM))
	kb, _ := pem.Decode([]byte(caKeyPEM))
	if cb == nil || kb == nil {
		return "", "", errors.New("autorité de certification illisible")
	}
	ca, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return "", "", err
	}
	caKey, err := x509.ParsePKCS1PrivateKey(kb.Bytes)
	if err != nil {
		return "", "", err
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}
	tpl := &x509.Certificate{SerialNumber: serial(), Subject: pkix.Name{CommonName: cn}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().AddDate(1, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	for _, n := range append([]string{cn}, names...) {
		if ip := net.ParseIP(n); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else if n != "" && !slices.Contains(tpl.DNSNames, n) {
			tpl.DNSNames = append(tpl.DNSNames, n)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return "", "", err
	}
	return pemBlock("CERTIFICATE", der), pemBlock("RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key)), nil
}

// ServerCA renvoie l'autorité de certification du serveur (créée au premier besoin).
func (r *Runner) ServerCA(ctx context.Context, sv *store.Server) (certPEM, keyPEM string, err error) {
	return EnsureServerCA(ctx, r.Store, r.Box, sv)
}

// EnsureServerCA lit ou crée l'autorité de certification d'un serveur (clé chiffrée en base).
func EnsureServerCA(ctx context.Context, st *store.Store, box *crypto.Box, sv *store.Server) (certPEM, keyPEM string, err error) {
	ca, err := st.ServerCA(ctx, sv.ID)
	if errors.Is(err, store.ErrNotFound) {
		cert, key, err := GenerateCA("Nodra CA " + sv.Name)
		if err != nil {
			return "", "", err
		}
		enc, err := box.Encrypt(key)
		if err != nil {
			return "", "", err
		}
		if err := st.SaveServerCA(ctx, sv.ID, cert, enc); err != nil {
			return "", "", err
		}
		ca, err = st.ServerCA(ctx, sv.ID) // relu : une création concurrente l'emporte
		if err != nil {
			return "", "", err
		}
	} else if err != nil {
		return "", "", err
	}
	key, err := box.Decrypt(ca.KeyEnc)
	return ca.CertPEM, key, err
}

// sslFiles : script qui écrit les certificats de la base dans <dir>/ssl (ou les retire si TLS est désactivé).
func (r *Runner) sslFiles(ctx context.Context, db *store.Database, sv *store.Server, dir string) (string, error) {
	sslDir := dir + "/ssl"
	if !sslEnabled(db) {
		return "rm -rf " + ssh.Quote(sslDir) + "\n", nil
	}
	caCert, caKey, err := r.ServerCA(ctx, sv)
	if err != nil {
		return "", fmt.Errorf("autorité de certification : %w", err)
	}
	cert, key, err := IssueCert(caCert, caKey, db.ID, []string{"localhost", "127.0.0.1", sv.IP})
	if err != nil {
		return "", err
	}
	return "rm -rf " + ssh.Quote(sslDir) + "\n" +
		writeFile(sslDir+"/ca.crt", caCert, "644") + writeFile(sslDir+"/ca.pem", caCert, "644") +
		writeFile(sslDir+"/server.crt", cert, "644") + writeFile(sslDir+"/server.key", key, "600") +
		writeFile(sslDir+"/server.pem", key+cert, "600"), nil
}

// sslOwnership : attribue les fichiers au compte du moteur (clé privée lisible par lui seul).
func sslOwnership(db *store.Database, base, dir string) string {
	spec := SSLTypes[db.Type]
	files := []string{"server.key", "server.crt", "server.pem"}
	if spec.Image {
		var in []string
		for _, f := range files {
			in = append(in, ssh.Quote(spec.Dir+"/"+f))
		}
		return base + " run --rm --no-deps --user root --entrypoint chown database " + spec.Owner + " " + strings.Join(in, " ") + " < /dev/null\n"
	}
	var on []string
	for _, f := range files {
		on = append(on, ssh.Quote(dir+"/ssl/"+f))
	}
	return "chown " + spec.Owner + " " + strings.Join(on, " ") + "\n"
}
