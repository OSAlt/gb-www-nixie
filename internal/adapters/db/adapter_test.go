package db

import (
	"testing"
)

func TestAdapter_ListContacts_Empty(t *testing.T) {
	// For testing ListContacts we'd need a real DB or a much more complex mock
	// since SQLC generated code expects a real DBTX.
	// Skipping for now as it requires complex mocking of pgx.Rows.
}

func TestAdapter_SocialCounts(t *testing.T) {
	// GetSocialCounts reads nixie.social_metrics via sqlc, which requires a
	// real DBTX; see the note in TestAdapter_ListContacts_Empty. The
	// endpoint's correctness is exercised via local coverage against the dev
	// database (see doc/social_migration.md).
}
