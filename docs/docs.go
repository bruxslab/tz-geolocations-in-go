// Package docs embeds the served OpenAPI contract.
package docs

import _ "embed"

//go:embed openapi.yaml
var OpenAPI []byte
