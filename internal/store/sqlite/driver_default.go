//go:build !android

package sqlite

import _ "modernc.org/sqlite"

const sqliteDriverName = "sqlite"
