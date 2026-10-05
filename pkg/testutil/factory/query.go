package factory

import (
	"context"

	"github.com/can3p/pcom/pkg/repo"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

// insert stores a new row; defaults and timestamps come back into m.
func insert(ctx context.Context, exec boil.ContextExecutor, m any) error {
	_, err := repo.Query(exec).NewInsert().Model(m).Exec(ctx)

	return err
}

// insertRow is insert for a builder that returns the row it made.
func insertRow[M any](ctx context.Context, exec boil.ContextExecutor, m *M) (*M, error) {
	if err := insert(ctx, exec, m); err != nil {
		return nil, err
	}

	return m, nil
}

// find loads the row whose primary key m holds, or sql.ErrNoRows.
func find[M any](ctx context.Context, exec boil.ContextExecutor, m *M) (*M, error) {
	if err := repo.Query(exec).NewSelect().Model(m).WherePK().Scan(ctx); err != nil {
		return nil, err
	}

	return m, nil
}

// one loads the first row where query holds, or sql.ErrNoRows.
func one[M any](ctx context.Context, exec boil.ContextExecutor, query string, args ...any) (*M, error) {
	m := new(M)
	if err := repo.Query(exec).NewSelect().Model(m).Where(query, args...).Limit(1).Scan(ctx); err != nil {
		return nil, err
	}

	return m, nil
}

// list loads every row where query holds.
func list[M any](ctx context.Context, exec boil.ContextExecutor, query string, args ...any) ([]*M, error) {
	var rows []*M
	if err := repo.Query(exec).NewSelect().Model(&rows).Where(query, args...).Scan(ctx); err != nil {
		return nil, err
	}

	return rows, nil
}

// exists reports whether a row of M where query holds exists.
func exists[M any](ctx context.Context, exec boil.ContextExecutor, query string, args ...any) (bool, error) {
	return repo.Query(exec).NewSelect().Model((*M)(nil)).Where(query, args...).Exists(ctx)
}
