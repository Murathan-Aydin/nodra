package store

import (
	"context"
	"fmt"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
)

// ScopeChain renvoie la chaîne de portées de la cible jusqu'à l'organisation racine
// (ex. ressource → environnement → projet → équipe → équipe parente).
// Les types projet/environnement/serveur/ressource sont branchés au fil des étapes suivantes.
func (s *Store) ScopeChain(ctx context.Context, sc rbac.Scope) ([]rbac.Scope, error) {
	switch sc.Type {
	case rbac.ScopeTeam:
		ids, err := s.TeamChain(ctx, sc.ID)
		if err != nil {
			return nil, err
		}
		out := make([]rbac.Scope, len(ids))
		for i, id := range ids {
			out[i] = rbac.Scope{Type: rbac.ScopeTeam, ID: id}
		}
		return out, nil
	default:
		if parent, ok := scopeParents[sc.Type]; ok {
			p, err := parent(ctx, s, sc.ID)
			if err != nil {
				return nil, err
			}
			rest, err := s.ScopeChain(ctx, p)
			if err != nil {
				return nil, err
			}
			return append([]rbac.Scope{sc}, rest...), nil
		}
		return nil, fmt.Errorf("type de portée non pris en charge : %q", sc.Type)
	}
}

// scopeParents : pour chaque type de portée, la fonction qui renvoie sa portée parente.
// Chaque module (projets, serveurs, ressources) s'y enregistre.
var scopeParents = map[string]func(context.Context, *Store, string) (rbac.Scope, error){}
