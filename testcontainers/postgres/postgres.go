package postgres

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	migrate "github.com/rubenv/sql-migrate"
)

var (
	containerOnce    sync.Once
	sharedContainer  *containerInstance
	containerInitErr error
	dbCounter        atomic.Uint64
)

type containerInstance struct {
	pool        *dockertest.Pool
	resource    *dockertest.Resource
	hostAndPort string
}

type TestDB struct {
	DB      *sqlx.DB
	dbName  string
	adminDB *sql.DB
}

func (t *TestDB) Close() error {
	if t.DB != nil {
		if err := t.DB.Close(); err != nil {
			return err
		}
	}

	if t.adminDB != nil && t.dbName != "" {
		_, err := t.adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", t.dbName))
		if err != nil {
			return fmt.Errorf("failed to drop database %s: %w", t.dbName, err)
		}
	}

	if t.adminDB != nil {
		if err := t.adminDB.Close(); err != nil {
			return err
		}
	}

	return nil
}

func Cleanup() error {
	if sharedContainer != nil && sharedContainer.pool != nil && sharedContainer.resource != nil {
		if err := sharedContainer.pool.Purge(sharedContainer.resource); err != nil {
			return fmt.Errorf("failed to purge container: %w", err)
		}
		sharedContainer = nil
	}
	return nil
}

func getOrCreateContainer() (*containerInstance, error) {
	containerOnce.Do(func() {
		pool, err := dockertest.NewPool("")
		if err != nil {
			containerInitErr = fmt.Errorf("could not construct pool: %w", err)
			return
		}

		err = pool.Client.Ping()
		if err != nil {
			containerInitErr = fmt.Errorf("could not connect to Docker: %w", err)
			return
		}

		resource, err := pool.RunWithOptions(&dockertest.RunOptions{
			Repository: "postgres",
			Tag:        "16-alpine",
			Env: []string{
				"POSTGRES_PASSWORD=secret",
				"POSTGRES_USER=testuser",
				"POSTGRES_DB=postgres",
				"listen_addresses = '*'",
			},
		}, func(config *docker.HostConfig) {
			config.AutoRemove = true
			config.RestartPolicy = docker.RestartPolicy{Name: "no"}
		})
		if err != nil {
			containerInitErr = fmt.Errorf("could not start resource: %w", err)
			return
		}

		hostAndPort := resource.GetHostPort("5432/tcp")
		_ = resource.Expire(300)

		var testConn *sql.DB
		if err = pool.Retry(func() error {
			testConn, err = sql.Open("postgres", fmt.Sprintf("postgres://testuser:secret@%s/postgres?sslmode=disable", hostAndPort))
			if err != nil {
				return err
			}
			defer func() { _ = testConn.Close() }()
			return testConn.Ping()
		}); err != nil {
			_ = pool.Purge(resource)
			containerInitErr = fmt.Errorf("could not connect to database: %w", err)
			return
		}

		log.Printf("Started shared PostgreSQL container at %s", hostAndPort)

		sharedContainer = &containerInstance{
			pool:        pool,
			resource:    resource,
			hostAndPort: hostAndPort,
		}
	})

	if containerInitErr != nil {
		return nil, containerInitErr
	}

	return sharedContainer, nil
}

func NewTestDB() (*TestDB, error) {
	container, err := getOrCreateContainer()
	if err != nil {
		return nil, err
	}

	dbNum := dbCounter.Add(1)
	dbName := fmt.Sprintf("testdb_%s_%d", uuid.New().String()[:8], dbNum)

	adminURL := fmt.Sprintf("postgres://testuser:secret@%s/postgres?sslmode=disable", container.hostAndPort)
	adminDB, err := sql.Open("postgres", adminURL)
	if err != nil {
		return nil, fmt.Errorf("could not connect to admin database: %w", err)
	}

	_, err = adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		_ = adminDB.Close()
		return nil, fmt.Errorf("could not create database %s: %w", dbName, err)
	}

	dbURL := fmt.Sprintf("postgres://testuser:secret@%s/%s?sslmode=disable", container.hostAndPort, dbName)
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		_ = adminDB.Close()
		return nil, fmt.Errorf("could not connect to test database: %w", err)
	}

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		_ = db.Close()
		_ = adminDB.Close()
		return nil, fmt.Errorf("could not get caller information")
	}
	migrationsDir := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")

	migrations := &migrate.FileMigrationSource{
		Dir: migrationsDir,
	}

	n, err := migrate.Exec(db, "postgres", migrations, migrate.Up)
	if err != nil {
		_ = db.Close()
		_ = adminDB.Close()
		return nil, fmt.Errorf("could not run migrations: %w", err)
	}

	log.Printf("Created database %s and applied %d migrations", dbName, n)

	sqlxDB := sqlx.NewDb(db, "postgres")

	return &TestDB{
		DB:      sqlxDB,
		dbName:  dbName,
		adminDB: adminDB,
	}, nil
}
