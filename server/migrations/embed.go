// Package migrations holds the ordered SQL schema files and embeds them into the
// binary. Keeping them in their own package lets the db package apply them via
// migrations.FS while the .sql files stay in this canonical, discoverable folder.
package migrations

import "embed"

// FS contains every *.sql migration, embedded at build time.
//
//go:embed *.sql
var FS embed.FS
