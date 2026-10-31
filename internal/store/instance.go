package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// InstanceSettings : réglages globaux de l'instance (ligne unique).
type InstanceSettings struct {
	FQDN                 *string    `json:"fqdn"`
	InstanceName         string     `json:"instance_name"`
	PublicIPv4           string     `json:"public_ipv4"`
	PublicIPv6           string     `json:"public_ipv6"`
	Timezone             string     `json:"timezone"`
	RegistrationEnabled  bool       `json:"registration_enabled"`
	AutoUpdateEnabled    bool       `json:"auto_update_enabled"`
	AutoUpdateFrequency  string     `json:"auto_update_frequency"`
	UpdateCheckFrequency string     `json:"update_check_frequency"`
	DNSValidationEnabled bool       `json:"dns_validation_enabled"`
	DNSServers           []string   `json:"dns_servers"`
	APIEnabled           bool       `json:"api_enabled"`
	APIAllowedIPs        []string   `json:"api_allowed_ips"`
	LatestVersion        string     `json:"latest_version"`
	UpdateCheckedAt      *time.Time `json:"update_checked_at"`
}

func (s *Store) InstanceSettings(ctx context.Context) (*InstanceSettings, error) {
	i := &InstanceSettings{}
	err := s.Pool.QueryRow(ctx, `SELECT fqdn, instance_name, public_ipv4, public_ipv6, timezone, registration_enabled,
		auto_update_enabled, auto_update_frequency, update_check_frequency, dns_validation_enabled, dns_servers, api_enabled,
		api_allowed_ips, latest_version, update_checked_at FROM instance_settings WHERE id = 1`).
		Scan(&i.FQDN, &i.InstanceName, &i.PublicIPv4, &i.PublicIPv6, &i.Timezone, &i.RegistrationEnabled, &i.AutoUpdateEnabled,
			&i.AutoUpdateFrequency, &i.UpdateCheckFrequency, &i.DNSValidationEnabled, &i.DNSServers, &i.APIEnabled, &i.APIAllowedIPs,
			&i.LatestVersion, &i.UpdateCheckedAt)
	return i, err
}

// UpdateInstanceSettings enregistre les réglages modifiables (la dernière version connue est gérée à part).
func (s *Store) UpdateInstanceSettings(ctx context.Context, i *InstanceSettings) error {
	_, err := s.Pool.Exec(ctx, `UPDATE instance_settings SET fqdn = $1, instance_name = $2, public_ipv4 = $3, public_ipv6 = $4,
		timezone = $5, registration_enabled = $6, auto_update_enabled = $7, auto_update_frequency = $8, update_check_frequency = $9,
		dns_validation_enabled = $10, dns_servers = $11, api_enabled = $12, api_allowed_ips = $13, updated_at = now() WHERE id = 1`,
		i.FQDN, i.InstanceName, i.PublicIPv4, i.PublicIPv6, i.Timezone, i.RegistrationEnabled, i.AutoUpdateEnabled,
		i.AutoUpdateFrequency, i.UpdateCheckFrequency, i.DNSValidationEnabled, i.DNSServers, i.APIEnabled, i.APIAllowedIPs)
	return err
}

// SetLatestVersion mémorise le résultat de la vérification des mises à jour.
func (s *Store) SetLatestVersion(ctx context.Context, v string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE instance_settings SET latest_version = $1, update_checked_at = now() WHERE id = 1`, v)
	return err
}

// LocalhostTarget : équipe personnelle du premier administrateur, où créer le serveur localhost.
// ok est faux si le serveur a déjà été créé une fois ou s'il n'y a pas encore d'administrateur.
func (s *Store) LocalhostTarget(ctx context.Context) (teamID, userID string, ok bool, err error) {
	var done bool
	if err = s.Pool.QueryRow(ctx, `SELECT localhost_bootstrapped FROM instance_settings WHERE id = 1`).Scan(&done); err != nil || done {
		return "", "", false, err
	}
	err = s.Pool.QueryRow(ctx, `SELECT t.id::text, u.id::text FROM users u
		JOIN team_members m ON m.user_id = u.id JOIN teams t ON t.id = m.team_id AND t.personal
		WHERE u.is_instance_admin ORDER BY u.created_at LIMIT 1`).Scan(&teamID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	return teamID, userID, err == nil, err
}

// MarkLocalhost enregistre le serveur localhost ; false si un autre processus l'a déjà fait.
func (s *Store) MarkLocalhost(ctx context.Context, serverID string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE instance_settings SET localhost_server_id = $1, localhost_bootstrapped = true
		WHERE id = 1 AND NOT localhost_bootstrapped`, serverID)
	return tag.RowsAffected() == 1, err
}

// LocalhostServerID : serveur localhost s'il existe encore ("" sinon).
func (s *Store) LocalhostServerID(ctx context.Context) (string, error) {
	var id *string
	err := s.Pool.QueryRow(ctx, `SELECT localhost_server_id::text FROM instance_settings WHERE id = 1`).Scan(&id)
	if err != nil || id == nil {
		return "", err
	}
	return *id, nil
}
