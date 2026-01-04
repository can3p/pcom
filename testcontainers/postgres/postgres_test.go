package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTestDB(t *testing.T) {
	testDB, err := NewTestDB()
	require.NoError(t, err)
	defer testDB.Close()

	var result int
	err = testDB.DB.Get(&result, "SELECT 1")
	require.NoError(t, err)
	assert.Equal(t, 1, result)

	var count int
	err = testDB.DB.Get(&count, "SELECT COUNT(*) FROM users")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
