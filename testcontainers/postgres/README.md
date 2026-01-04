# PostgreSQL Test Container Helper

This package provides a simple test helper for spinning up PostgreSQL containers with migrations applied.

## Features

- **Singleton container pattern**: Starts one PostgreSQL container shared across all tests
- **Per-test database isolation**: Each test gets its own database with migrations applied
- **Parallel test support**: Tests can run in parallel without interfering with each other
- Uses `github.com/ory/dockertest` for container management
- Automatically applies all migrations from the `migrations/` folder
- Provides a `*sqlx.DB` handle for testing
- Simple one-liner setup with cleanup

## Usage

### Basic Usage

```go
package mypackage_test

import (
    "testing"
    "github.com/can3p/pcom/testcontainers/postgres"
    "github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
    code := m.Run()
    postgres.Cleanup() // Cleanup container after all tests
    if code != 0 {
        panic(code)
    }
}

func TestMyFunction(t *testing.T) {
    // Get a test database with all migrations applied
    testDB, err := postgres.NewTestDB()
    require.NoError(t, err)
    defer testDB.Close() // Drops the database (not the container)
    
    // Use testDB.DB (*sqlx.DB) for your tests
    var count int
    err = testDB.DB.Get(&count, "SELECT COUNT(*) FROM users")
    require.NoError(t, err)
    
    // Your test logic here...
}
```

### Parallel Tests

```go
func TestParallelOperations(t *testing.T) {
    t.Run("test1", func(t *testing.T) {
        t.Parallel()
        testDB, err := postgres.NewTestDB()
        require.NoError(t, err)
        defer testDB.Close()
        
        // Each test gets its own isolated database
        // Tests can run in parallel without conflicts
    })
    
    t.Run("test2", func(t *testing.T) {
        t.Parallel()
        testDB, err := postgres.NewTestDB()
        require.NoError(t, err)
        defer testDB.Close()
        
        // Completely isolated from test1
    })
}
```

## How It Works

1. **First call to `NewTestDB()`**: Starts a shared PostgreSQL container (happens once)
2. **Each `NewTestDB()` call**: Creates a new database with a unique name and applies all migrations
3. **`TestDB.Close()`**: Drops the test database (container keeps running)
4. **`Cleanup()`**: Stops and removes the container (call in `TestMain`)

## Requirements

- Docker must be running on the host machine
- The helper will automatically:
  - Pull the `postgres:16-alpine` image if not present
  - Start a single shared container on first use
  - Create isolated databases for each test
  - Apply all migrations from `migrations/` folder
  - Set container to auto-remove after 300 seconds

## API

### `NewTestDB() (*TestDB, error)`

Creates a new isolated database with migrations applied. Uses a singleton container that is created on first call.

Returns:
- `*TestDB`: Database handle with connection
- `error`: Any error during setup

### `TestDB.DB`

A `*sqlx.DB` handle connected to the test database.

### `TestDB.Close() error`

Drops the test database and closes connections. The container continues running for other tests. Should be called with `defer` after creating the test DB.

### `Cleanup() error`

Stops and removes the shared PostgreSQL container. Should be called in `TestMain` after all tests complete.
