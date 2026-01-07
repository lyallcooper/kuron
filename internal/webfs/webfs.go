// Package webfs provides the embedded web assets (templates and static files).
package webfs

import "embed"

//go:embed all:static all:templates
var FS embed.FS
