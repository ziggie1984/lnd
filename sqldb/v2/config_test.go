package sqldb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSqliteConfigMaxConns verifies that SQLite keeps the low default
// connection limit unless the caller overrides it explicitly.
func TestSqliteConfigMaxConns(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		maxConns     int
		expectedConn int
	}{
		{
			name:         "default limit",
			expectedConn: DefaultSqliteMaxConns,
		},
		{
			name:         "explicit limit",
			maxConns:     7,
			expectedConn: 7,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			cfg := &SqliteConfig{
				MaxConnections: testCase.maxConns,
			}

			require.Equal(t, testCase.expectedConn, cfg.MaxConns())
		})
	}
}
