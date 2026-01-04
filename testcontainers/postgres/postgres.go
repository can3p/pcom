package postgres

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"runtime"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	migrate "github.com/rubenv/sql-migrate"
)

type TestDB struct {
	DB       *sqlx.DB
	pool     *dockertest.Pool
	resource *dockertest.Resource
}

func (t *TestDB) Close() error {
	if t.DB != nil {
		if err := t.DB.Close(); err != nil {
			return err
		}
	}
	if t.pool != nil && t.resource != nil {
		if err := t.pool.Purge(t.resource); err != nil {
			return fmt.Errorf("failed to purge resource: %w", err)
		}
	}
	return nil
}

func NewTestDB() (*TestDB, error) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		return nil, fmt.Errorf("could not construct pool: %w", err)
	}

	err = pool.Client.Ping()
	if err != nil {
		return nil, fmt.Errorf("could not connect to Docker: %w", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "16-alpine",
		Env: []string{
			"POSTGRES_PASSWORD=secret",
			"POSTGRES_USER=testuser",
			"POSTGRES_DB=testdb",
			"listen_addresses = '*'",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		return nil, fmt.Errorf("could not start resource: %w", err)
	}

	hostAndPort := resource.GetHostPort("5432/tcp")
	databaseUrl := fmt.Sprintf("postgres://testuser:secret@%s/testdb?sslmode=disable", hostAndPort)

	resource.Expire(120)

	var db *sql.DB
	if err = pool.Retry(func() error {
		db, err = sql.Open("postgres", databaseUrl)
		if err != nil {
			return err
		}
		return db.Ping()
	}); err != nil {
		pool.Purge(resource)
		return nil, fmt.Errorf("could not connect to database: %w", err)
	}

	sqlxDB := sqlx.NewDb(db, "postgres")

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		pool.Purge(resource)
		return nil, fmt.Errorf("could not get caller information")
	}
	migrationsDir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")

	migrations := &migrate.FileMigrationSource{
		Dir: migrationsDir,
	}

	n, err := migrate.Exec(db, "postgres", migrations, migrate.Up)
	if err != nil {
		pool.Purge(resource)
		return nil, fmt.Errorf("could not run migrations: %w", err)
	}

	log.Printf("Applied %d migrations to test database", n)

	return &TestDB{
		DB:       sqlxDB,
		pool:     pool,
		resource: resource,
	}, nil
}
