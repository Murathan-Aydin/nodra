package store

import "context"

type InstanceSettings struct {
	FQDN                *string `json:"fqdn"`
	RegistrationEnabled bool    `json:"registration_enabled"`
	AutoUpdateEnabled   bool    `json:"auto_update_enabled"`
}

func (s *Store) InstanceSettings(ctx context.Context) (*InstanceSettings, error) {
	i := &InstanceSettings{}
	err := s.Pool.QueryRow(ctx, `SELECT fqdn, registration_enabled, auto_update_enabled FROM instance_settings WHERE id = 1`).
		Scan(&i.FQDN, &i.RegistrationEnabled, &i.AutoUpdateEnabled)
	return i, err
}
