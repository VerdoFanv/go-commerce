// Package migrations embeds the versioned SQL migrations so the binaries are
// self-contained: `api` and `worker` can apply/inspect schema without shipping
// separate files into the container image.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
