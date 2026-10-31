// Package docs embarque le contrat de l'API (API.md) pour générer la spécification OpenAPI servie par Nodra.
package docs

import _ "embed"

//go:embed API.md
var API string
