package sqldb

import (
	"io"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

// TestPostgresSchemaReplacements verifies that the Postgres schema
// replacements do not rewrite SQL keywords that only contain a replacement
// token as a substring.
func TestPostgresSchemaReplacements(t *testing.T) {
	t.Parallel()

	postgresFS := newReplacerFS(fstest.MapFS{
		"schema.sql": &fstest.MapFile{
			Data: []byte("created_at TIMESTAMP NOT NULL DEFAULT " +
				"CURRENT_TIMESTAMP"),
		},
	}, postgresSchemaReplacements)

	file, err := postgresFS.Open("schema.sql")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, file.Close())
	})

	content, err := io.ReadAll(file)
	require.NoError(t, err)

	require.Equal(t,
		"created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT "+
			"CURRENT_TIMESTAMP", string(content),
	)
}
