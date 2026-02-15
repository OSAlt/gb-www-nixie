//go:build tools

package tools

import (
	_ "github.com/a-h/templ/cmd/templ"
	_ "github.com/magefile/mage"
	_ "github.com/pressly/goose/v3/cmd/goose"
	_ "github.com/sqlc-dev/sqlc/cmd/sqlc"
)
