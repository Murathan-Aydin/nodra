package backup

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/Murathan-Aydin/nodra/internal/crypto"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// ClientImage : client S3 lancé sur les serveurs pour envoyer et récupérer les sauvegardes
// (rclone, configuré par variables d'environnement ; les images MinIO ne sont plus publiées sur Docker Hub).
const ClientImage = "rclone/rclone:1"

// S3 : identifiants déchiffrés d'un stockage.
type S3 struct {
	Endpoint, Region, Bucket, Prefix, AccessKey, SecretKey string
}

func Decrypt(box *crypto.Box, s *store.S3Storage) (S3, error) {
	ak, err := box.Decrypt(s.AccessKeyEnc)
	if err != nil {
		return S3{}, err
	}
	sk, err := box.Decrypt(s.SecretKeyEnc)
	if err != nil {
		return S3{}, err
	}
	return S3{Endpoint: s.Endpoint, Region: s.Region, Bucket: s.Bucket, Prefix: strings.Trim(s.PathPrefix, "/"), AccessKey: ak, SecretKey: sk}, nil
}

func (s S3) client() (*minio.Client, error) {
	u, err := url.Parse(s.Endpoint)
	if err != nil || u.Host == "" {
		return nil, errors.New("endpoint S3 invalide")
	}
	return minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(s.AccessKey, s.SecretKey, ""), Secure: u.Scheme == "https",
		Region: s.Region})
}

// Key construit la clé d'un objet sous le préfixe du stockage.
func (s S3) Key(parts ...string) string {
	if s.Prefix != "" {
		parts = append([]string{s.Prefix}, parts...)
	}
	return path.Join(parts...)
}

// Check vérifie l'accès au bucket.
func (s S3) Check(ctx context.Context) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ok, err := c.BucketExists(ctx, s.Bucket)
	if err != nil {
		return fmt.Errorf("accès S3 impossible : %w", err)
	}
	if !ok {
		return fmt.Errorf("le bucket %s n'existe pas", s.Bucket)
	}
	return nil
}

func (s S3) Remove(ctx context.Context, key string) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	return c.RemoveObject(ctx, s.Bucket, key, minio.RemoveObjectOptions{})
}

// Presign renvoie une URL de téléchargement temporaire.
func (s S3) Presign(ctx context.Context, key string, ttl time.Duration) (string, error) {
	c, err := s.client()
	if err != nil {
		return "", err
	}
	u, err := c.PresignedGetObject(ctx, s.Bucket, key, ttl, url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// rcloneEnv : configuration du remote « nodra » par variables d'environnement (jamais en argument).
func (s S3) rcloneEnv() []string {
	u, _ := url.Parse(s.Endpoint)
	provider := "Other" // style « chemin » : MinIO, Garage, SeaweedFS, R2…
	if strings.HasSuffix(u.Hostname(), "amazonaws.com") {
		provider = "AWS"
	}
	return []string{"RCLONE_CONFIG_NODRA_TYPE=s3", "RCLONE_CONFIG_NODRA_PROVIDER=" + provider,
		"RCLONE_CONFIG_NODRA_ACCESS_KEY_ID=" + s.AccessKey, "RCLONE_CONFIG_NODRA_SECRET_ACCESS_KEY=" + s.SecretKey,
		"RCLONE_CONFIG_NODRA_ENDPOINT=" + s.Endpoint, "RCLONE_CONFIG_NODRA_REGION=" + s.Region}
}

// rclone construit le script : variables exportées puis transmises au conteneur par leur seul nom.
func (s S3) rclone(mount, args string) string {
	var b strings.Builder
	var names []string
	for _, kv := range s.rcloneEnv() {
		k, v, _ := strings.Cut(kv, "=")
		fmt.Fprintf(&b, "export %s=%s\n", k, ssh.Quote(v))
		names = append(names, "-e "+k)
	}
	fmt.Fprintf(&b, "docker run --rm %s -v %s %s %s\n", strings.Join(names, " "), mount, ClientImage, args)
	return b.String()
}

// UploadScript envoie file vers la clé key.
func (s S3) UploadScript(file, key string) string {
	dir := file[:strings.LastIndex(file, "/")]
	return s.rclone(ssh.Quote(dir+":"+dir+":ro"), "copyto --s3-no-check-bucket "+ssh.Quote(file)+" "+ssh.Quote("nodra:"+s.Bucket+"/"+key))
}

// DownloadScript récupère la clé key dans file.
func (s S3) DownloadScript(key, file string) string {
	dir := file[:strings.LastIndex(file, "/")]
	return "mkdir -p " + ssh.Quote(dir) + "\n" + s.rclone(ssh.Quote(dir+":"+dir), "copyto "+ssh.Quote("nodra:"+s.Bucket+"/"+key)+" "+ssh.Quote(file))
}
