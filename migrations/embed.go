package migrations

import "embed"

// Files are embedded so migrations do not depend on the working directory.
//
//go:embed *.sql
var Files embed.FS
