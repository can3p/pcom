package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	code := m.Run()
	Cleanup()
	if code != 0 {
		panic(code)
	}
}

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

func TestParallelDatabases(t *testing.T) {
	t.Run("db1", func(t *testing.T) {
		t.Parallel()
		testDB, err := NewTestDB()
		require.NoError(t, err)
		defer testDB.Close()

		_, err = testDB.DB.Exec("INSERT INTO users (id, email, timezone, username) VALUES ($1, $2, $3, $4)",
			"00000000-0000-0000-0000-000000000001", "test1@example.com", "UTC", "user1")
		require.NoError(t, err)

		var count int
		err = testDB.DB.Get(&count, "SELECT COUNT(*) FROM users")
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("db2", func(t *testing.T) {
		t.Parallel()
		testDB, err := NewTestDB()
		require.NoError(t, err)
		defer testDB.Close()

		var count int
		err = testDB.DB.Get(&count, "SELECT COUNT(*) FROM users")
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("db3", func(t *testing.T) {
		t.Parallel()
		testDB, err := NewTestDB()
		require.NoError(t, err)
		defer testDB.Close()

		_, err = testDB.DB.Exec("INSERT INTO users (id, email, timezone, username) VALUES ($1, $2, $3, $4)",
			"00000000-0000-0000-0000-000000000002", "test2@example.com", "UTC", "user2")
		require.NoError(t, err)

		var count int
		err = testDB.DB.Get(&count, "SELECT COUNT(*) FROM users")
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})
}
