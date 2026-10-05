// Package model holds pcom's database rows: one struct per table, written by
// hand. They are bun models, but their bun tags mean nothing outside
// pkg/repo, the only package that runs queries.
//
// Conventions: a nullable column is a pointer; a relation is a plain field,
// filled only when a repository method loads it; a column with a database
// default has a default tag, so its zero value inserts DEFAULT and the value
// comes back after the insert. TestModels_MatchTheSchema keeps the structs
// and the migrations in step.
package model

import (
	"time"

	"github.com/uptrace/bun"
)

// stampTimes stamps a row's times on every write: an insert sets created_at
// and updated_at when they are zero, an update sets updated_at.
// Each argument is a *time.Time, a **time.Time or nil.
func stampTimes(query bun.Query, createdAt, updatedAt any) {
	now := time.Now().UTC()

	switch query.(type) {
	case *bun.InsertQuery:
		setIfZero(createdAt, now)
		setIfZero(updatedAt, now)
	case *bun.UpdateQuery:
		setTime(updatedAt, now)
	}
}

func setIfZero(field any, now time.Time) {
	switch v := field.(type) {
	case *time.Time:
		if v.IsZero() {
			*v = now
		}
	case **time.Time:
		if *v == nil || (*v).IsZero() {
			*v = &now
		}
	}
}

func setTime(field any, now time.Time) {
	switch v := field.(type) {
	case *time.Time:
		*v = now
	case **time.Time:
		*v = &now
	}
}
