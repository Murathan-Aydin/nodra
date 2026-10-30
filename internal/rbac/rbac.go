// Package rbac définit le catalogue des permissions, les rôles système et la résolution
// des droits (portée la plus spécifique gagnante, refus prioritaire à portée égale).
package rbac

import (
	"slices"
	"strings"
)

type Permission struct {
	Key         string `json:"key"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

// Catalog liste toutes les permissions atomiques. Les rôles peuvent aussi utiliser
// les jokers "*" (tout) et "préfixe.*" (ex. "resource.*").
var Catalog = []Permission{
	{"team.read", "Équipe", "Voir l'équipe, ses membres, groupes et rôles"},
	{"team.update", "Équipe", "Modifier le nom et la description de l'équipe"},
	{"team.delete", "Équipe", "Supprimer l'équipe"},
	{"team.subteams.manage", "Équipe", "Créer et supprimer des sous-équipes"},
	{"team.members.manage", "Équipe", "Inviter et retirer des membres, gérer les groupes"},
	{"team.roles.manage", "Équipe", "Créer des rôles et attribuer des permissions"},
	{"team.audit.read", "Équipe", "Consulter et exporter le journal d'audit"},
	{"approvals.manage", "Approbations", "Gérer les règles d'approbation"},
	{"approvals.approve", "Approbations", "Approuver ou rejeter les demandes"},
	{"project.read", "Projets", "Voir les projets et environnements"},
	{"project.create", "Projets", "Créer des projets"},
	{"project.update", "Projets", "Modifier les projets"},
	{"project.delete", "Projets", "Supprimer des projets"},
	{"environment.create", "Projets", "Créer des environnements"},
	{"environment.update", "Projets", "Modifier ou cloner des environnements"},
	{"environment.delete", "Projets", "Supprimer des environnements"},
	{"server.read", "Serveurs", "Voir les serveurs et leurs métriques"},
	{"server.create", "Serveurs", "Ajouter des serveurs"},
	{"server.update", "Serveurs", "Modifier, valider et entretenir les serveurs"},
	{"server.delete", "Serveurs", "Supprimer des serveurs"},
	{"server.terminal", "Serveurs", "Ouvrir un terminal sur les serveurs"},
	{"server.proxy.manage", "Serveurs", "Gérer le proxy et les certificats"},
	{"resource.read", "Ressources", "Voir les applications, bases et services"},
	{"resource.create", "Ressources", "Créer des ressources"},
	{"resource.update", "Ressources", "Modifier la configuration des ressources"},
	{"resource.delete", "Ressources", "Supprimer des ressources"},
	{"resource.deploy", "Ressources", "Déployer, démarrer, arrêter, redémarrer"},
	{"resource.env.read", "Ressources", "Lire les variables d'environnement et secrets"},
	{"resource.env.write", "Ressources", "Modifier les variables d'environnement"},
	{"resource.logs.read", "Ressources", "Lire les logs"},
	{"resource.terminal", "Ressources", "Ouvrir un terminal dans les conteneurs"},
	{"resource.backup.manage", "Ressources", "Configurer et lancer les sauvegardes"},
	{"resource.backup.restore", "Ressources", "Restaurer une sauvegarde"},
	{"source.read", "Intégrations", "Voir les sources Git"},
	{"source.manage", "Intégrations", "Gérer les sources Git"},
	{"key.read", "Intégrations", "Voir les clés SSH (sans la partie privée)"},
	{"key.manage", "Intégrations", "Gérer les clés SSH"},
	{"storage.read", "Intégrations", "Voir les stockages S3"},
	{"storage.manage", "Intégrations", "Gérer les stockages S3"},
	{"notification.manage", "Intégrations", "Configurer les notifications"},
	{"variable.read", "Intégrations", "Lire les variables partagées"},
	{"variable.manage", "Intégrations", "Gérer les variables partagées"},
}

// Types de portée, du plus large au plus précis.
const (
	ScopeTeam        = "team"
	ScopeProject     = "project"
	ScopeEnvironment = "environment"
	ScopeServer      = "server"
	ScopeResource    = "resource"
)

var ScopeTypes = []string{ScopeTeam, ScopeProject, ScopeEnvironment, ScopeServer, ScopeResource}

type Scope struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

const (
	Allow = "allow"
	Deny  = "deny"

	RoleOwner     = "owner"
	RoleAdmin     = "admin"
	RoleDeveloper = "developer"
	RoleViewer    = "viewer"
)

type Role struct {
	Key         string
	Name        string
	Description string
	Permissions []string
}

// SystemRoles : rôles fournis par Nodra, non modifiables.
func SystemRoles() []Role {
	var admin []string
	for _, p := range Catalog {
		if p.Key != "team.delete" {
			admin = append(admin, p.Key)
		}
	}
	return []Role{
		{RoleOwner, "Propriétaire", "Tous les droits, y compris la suppression de l'équipe", []string{"*"}},
		{RoleAdmin, "Administrateur", "Tous les droits sauf la suppression de l'équipe", admin},
		{RoleDeveloper, "Développeur", "Déploie et configure les ressources, sans administration de l'équipe", []string{
			"team.read", "project.read", "project.create", "project.update", "environment.create", "environment.update",
			"server.read", "resource.read", "resource.create", "resource.update", "resource.deploy", "resource.env.read",
			"resource.env.write", "resource.logs.read", "resource.terminal", "resource.backup.manage",
			"source.read", "key.read", "storage.read", "variable.read",
		}},
		{RoleViewer, "Lecteur", "Lecture seule, sans accès aux secrets", []string{
			"team.read", "project.read", "server.read", "resource.read", "resource.logs.read",
			"source.read", "key.read", "storage.read",
		}},
	}
}

func IsPermission(key string) bool {
	return slices.ContainsFunc(Catalog, func(p Permission) bool { return p.Key == key })
}

// ValidGrant accepte une permission du catalogue, "*" ou un joker "préfixe.*" couvrant au moins une permission.
func ValidGrant(g string) bool {
	if g == "*" || IsPermission(g) {
		return true
	}
	if pre, ok := strings.CutSuffix(g, "*"); ok && strings.HasSuffix(pre, ".") {
		return slices.ContainsFunc(Catalog, func(p Permission) bool { return strings.HasPrefix(p.Key, pre) })
	}
	return false
}

// Grants indique si un ensemble de permissions (avec jokers) couvre perm.
func Grants(granted []string, perm string) bool {
	for _, g := range granted {
		if g == "*" || g == perm {
			return true
		}
		if pre, ok := strings.CutSuffix(g, "*"); ok && strings.HasSuffix(pre, ".") && strings.HasPrefix(perm, pre) {
			return true
		}
	}
	return false
}

// Expand renvoie les permissions du catalogue couvertes par granted.
func Expand(granted []string) []string {
	var out []string
	for _, p := range Catalog {
		if Grants(granted, p.Key) {
			out = append(out, p.Key)
		}
	}
	return out
}

// Assignment : une attribution de rôle applicable à l'utilisateur (directement ou via un groupe).
type Assignment struct {
	Scope       Scope
	Effect      string
	RoleKey     string // clé du rôle système, vide pour un rôle personnalisé
	Permissions []string
}

// Decide applique les règles de résolution :
//  1. un propriétaire (rôle système owner, allow) sur une portée de la chaîne a tous les droits ;
//  2. sinon on parcourt la chaîne du plus précis au plus large : la première portée qui
//     a une décision pour perm l'emporte, et à portée égale le refus l'emporte ;
//  3. sans décision, refus.
//
// chain va de la portée cible vers la racine (ex. ressource, environnement, projet, équipe, équipe parente).
func Decide(chain []Scope, as []Assignment, perm string) bool {
	for _, a := range as {
		if a.RoleKey == RoleOwner && a.Effect == Allow && slices.Contains(chain, a.Scope) {
			return true
		}
	}
	for _, sc := range chain {
		allow, deny := false, false
		for _, a := range as {
			if a.Scope != sc || !Grants(a.Permissions, perm) {
				continue
			}
			if a.Effect == Deny {
				deny = true
			} else {
				allow = true
			}
		}
		if deny {
			return false
		}
		if allow {
			return true
		}
	}
	return false
}

// Effective renvoie toutes les permissions accordées sur la portée (tête de chain).
func Effective(chain []Scope, as []Assignment) []string {
	out := []string{}
	for _, p := range Catalog {
		if Decide(chain, as, p.Key) {
			out = append(out, p.Key)
		}
	}
	return out
}

// IsOwner indique si l'utilisateur est propriétaire sur l'une des portées de la chaîne.
func IsOwner(chain []Scope, as []Assignment) bool {
	return Decide(chain, as, "\x00owner-only") // seule la règle 1 peut accorder une permission inexistante
}

// Scopes de jetons API.
const (
	TokenRead   = "read"
	TokenWrite  = "write"
	TokenDeploy = "deploy"
	TokenRoot   = "root"
)

var TokenScopes = []string{TokenRead, TokenWrite, TokenDeploy, TokenRoot}

// TokenAllows restreint les droits d'un jeton API selon ses scopes (en plus du RBAC de son propriétaire) :
//   - read   : permissions *.read (hors secrets : resource.env.read exige write ou root) ;
//   - write  : tout sauf déploiement, administration de l'équipe et approbations ;
//   - deploy : resource.deploy ;
//   - root   : tout.
func TokenAllows(scopes []string, perm string) bool {
	for _, s := range scopes {
		switch s {
		case TokenRoot:
			return true
		case TokenRead:
			if strings.HasSuffix(perm, ".read") && perm != "resource.env.read" && perm != "variable.read" {
				return true
			}
		case TokenWrite:
			if perm != "resource.deploy" && !strings.HasPrefix(perm, "approvals.") &&
				(perm == "team.read" || !strings.HasPrefix(perm, "team.")) {
				return true
			}
		case TokenDeploy:
			if perm == "resource.deploy" {
				return true
			}
		}
	}
	return false
}

// GateableActions : actions qu'une règle d'approbation peut soumettre à validation.
var GateableActions = []string{
	"resource.deploy", "resource.delete", "resource.env.write", "resource.backup.restore",
	"environment.delete", "project.delete", "server.delete",
}
