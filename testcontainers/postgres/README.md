# PostgreSQL Test Container Helper

This package provides a simple test helper for spinning up PostgreSQL containers with migrations applied.

## Features

- Uses `github.com/ory/dockertest` for container management
- Automatically applies all migrations from the `migrations/` folder
- Provides a `*sqlx.DB` handle for testing
- Simple one-liner setup with cleanup

## Usage

```go
package mypackage_test

import (
    "testing"
    "github.com/can3p/pcom/testcontainers/postgres"
    "github.com/stretchr/testify/require"
)

func TestMyFunction(t *testing.T) {
    // Get a test database with all migrations applied
    testDB, err := postgres.NewTestDB()
    require.NoError(t, err)
    defer testDB.Close() // Cleanup container when done
    
    // Use testDB.DB (*sqlx.DB) for your tests
    var count int
    err = testDB.DB.Get(&count, "SELECT COUNT(*) FROM users")
    require.NoError(t, err)
    
    // Your test logic here...
}
```

## Requirements

- Docker must be running on the host machine
- The test will automatically:
  - Pull the `postgres:16-alpine` image if not present
  - Start a container with a test database
  - Apply all migrations from `migrations/` folder
  - Set container to auto-remove after 120 seconds

## API

### `NewTestDB() (*TestDB, error)`

Creates a new PostgreSQL test container with migrations applied.

Returns:
- `*TestDB`: Container handle with DB connection
- `error`: Any error during setup

### `TestDB.DB`

A `*sqlx.DB` handle connected to the test database.

### `TestDB.Close() error`

Stops and removes the container. Should be called with `defer` after creating the test DB.
