package model_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// models is every table's struct. A table the migrations add needs a struct
// here; TestModels_MatchTheSchema fails until it has one.
var models = []any{
	(*model.LoginAttempt)(nil),
	(*model.MediaUpload)(nil),
	(*model.NormalizedURL)(nil),
	(*model.OutgoingEmail)(nil),
	(*model.Post)(nil),
	(*model.PostComment)(nil),
	(*model.PostPrompt)(nil),
	(*model.PostShare)(nil),
	(*model.PostStat)(nil),
	(*model.RSSFeed)(nil),
	(*model.RSSItem)(nil),
	(*model.SystemSetting)(nil),
	(*model.User)(nil),
	(*model.UserAPIKey)(nil),
	(*model.UserConnection)(nil),
	(*model.UserConnectionMediationRequest)(nil),
	(*model.UserConnectionMediator)(nil),
	(*model.UserFeedItem)(nil),
	(*model.UserFeedSubscription)(nil),
	(*model.UserFeedToken)(nil),
	(*model.UserInvitation)(nil),
	(*model.UserProfile)(nil),
	(*model.UserSignupRequest)(nil),
	(*model.UserStyle)(nil),
	(*model.WhitelistedConnection)(nil),
}

// unmodeled tables belong to tools, not to pcom's data.
var unmodeled = map[string]bool{"migrations": true}

type column struct {
	nullable, defaulted, pk bool
}

// TestModels_MatchTheSchema replaces code generation: every struct in
// pkg/model must agree with the migrated database on its column set, which
// columns are nullable (pointer fields), which have a default (default tag)
// and the primary key, and every table must have a struct.
func TestModels_MatchTheSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	tdb := testdb.New(t)
	db := bun.NewDB(tdb.SQL, pgdialect.New())

	schema := map[string]map[string]column{}
	rows, err := db.QueryContext(ctx, `
		SELECT c.table_name, c.column_name, c.is_nullable = 'YES', c.column_default IS NOT NULL,
		       EXISTS (
		           SELECT 1 FROM information_schema.table_constraints tc
		           JOIN information_schema.key_column_usage k
		             ON k.constraint_name = tc.constraint_name AND k.table_name = tc.table_name
		           WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_name = c.table_name
		             AND k.column_name = c.column_name)
		FROM information_schema.columns c
		WHERE c.table_schema = 'public'`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var table, name string
		var c column
		require.NoError(t, rows.Scan(&table, &name, &c.nullable, &c.defaulted, &c.pk))
		if schema[table] == nil {
			schema[table] = map[string]column{}
		}
		schema[table][name] = c
	}
	require.NoError(t, rows.Err())

	modeled := map[string]bool{}
	for _, m := range models {
		typ := reflect.TypeOf(m).Elem()
		table := db.Table(typ)
		modeled[table.Name] = true

		cols, ok := schema[table.Name]
		if !ok {
			t.Errorf("%s: table %s does not exist", typ.Name(), table.Name)
			continue
		}

		seen := map[string]bool{}
		for _, f := range table.Fields {
			seen[f.Name] = true
			c, ok := cols[f.Name]
			if !ok {
				t.Errorf("%s.%s: column %s.%s does not exist", typ.Name(), f.GoName, table.Name, f.Name)
				continue
			}
			if c.nullable != f.IsPtr {
				t.Errorf("%s.%s: column nullable=%v, field pointer=%v", typ.Name(), f.GoName, c.nullable, f.IsPtr)
			}
			if c.defaulted != (f.SQLDefault != "") {
				t.Errorf("%s.%s: column has default=%v, field default tag=%q", typ.Name(), f.GoName, c.defaulted, f.SQLDefault)
			}
			if c.pk != f.IsPK {
				t.Errorf("%s.%s: column primary key=%v, field pk=%v", typ.Name(), f.GoName, c.pk, f.IsPK)
			}
		}
		for name := range cols {
			if !seen[name] {
				t.Errorf("%s: no field for column %s.%s", typ.Name(), table.Name, name)
			}
		}

		for sf := range typ.Fields() {
			if strings.HasPrefix(sf.Tag.Get("bun"), "rel:") {
				if _, ok := table.Relations[sf.Name]; !ok {
					t.Errorf("%s.%s: bun did not register the relation", typ.Name(), sf.Name)
				}
			}
		}
	}

	for table := range schema {
		if !modeled[table] && !unmodeled[table] {
			t.Errorf("table %s has no struct in pkg/model", table)
		}
	}

	for table, names := range map[string]any{"users": model.UserColumns, "login_attempts": model.LoginAttemptColumns} {
		v := reflect.ValueOf(names)
		for i := range v.NumField() {
			if _, ok := schema[table][v.Field(i).String()]; !ok {
				t.Errorf("%s: %s is not a column of %s", v.Type().Field(i).Name, v.Field(i).String(), table)
			}
		}
	}
}
