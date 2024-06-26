// Code generated by SQLBoiler 4.16.2 (https://github.com/volatiletech/sqlboiler). DO NOT EDIT.
// This file is meant to be re-generated in place and/or deleted at any time.

package core

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/friendsofgo/errors"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
	"github.com/volatiletech/sqlboiler/v4/queries/qmhelper"
	"github.com/volatiletech/strmangle"
)

// UserInvitation is an object representing the database table.
type UserInvitation struct {
	ID               string      `boil:"id" json:"id" toml:"id" yaml:"id"`
	UserID           string      `boil:"user_id" json:"user_id" toml:"user_id" yaml:"user_id"`
	InvitationEmail  null.String `boil:"invitation_email" json:"invitation_email,omitempty" toml:"invitation_email" yaml:"invitation_email,omitempty"`
	InvitationSentAt null.Time   `boil:"invitation_sent_at" json:"invitation_sent_at,omitempty" toml:"invitation_sent_at" yaml:"invitation_sent_at,omitempty"`
	CreatedAt        null.Time   `boil:"created_at" json:"created_at,omitempty" toml:"created_at" yaml:"created_at,omitempty"`
	UpdatedAt        null.Time   `boil:"updated_at" json:"updated_at,omitempty" toml:"updated_at" yaml:"updated_at,omitempty"`
	CreatedUserID    null.String `boil:"created_user_id" json:"created_user_id,omitempty" toml:"created_user_id" yaml:"created_user_id,omitempty"`

	R *userInvitationR `boil:"-" json:"-" toml:"-" yaml:"-"`
	L userInvitationL  `boil:"-" json:"-" toml:"-" yaml:"-"`
}

var UserInvitationColumns = struct {
	ID               string
	UserID           string
	InvitationEmail  string
	InvitationSentAt string
	CreatedAt        string
	UpdatedAt        string
	CreatedUserID    string
}{
	ID:               "id",
	UserID:           "user_id",
	InvitationEmail:  "invitation_email",
	InvitationSentAt: "invitation_sent_at",
	CreatedAt:        "created_at",
	UpdatedAt:        "updated_at",
	CreatedUserID:    "created_user_id",
}

var UserInvitationTableColumns = struct {
	ID               string
	UserID           string
	InvitationEmail  string
	InvitationSentAt string
	CreatedAt        string
	UpdatedAt        string
	CreatedUserID    string
}{
	ID:               "user_invitations.id",
	UserID:           "user_invitations.user_id",
	InvitationEmail:  "user_invitations.invitation_email",
	InvitationSentAt: "user_invitations.invitation_sent_at",
	CreatedAt:        "user_invitations.created_at",
	UpdatedAt:        "user_invitations.updated_at",
	CreatedUserID:    "user_invitations.created_user_id",
}

// Generated where

var UserInvitationWhere = struct {
	ID               whereHelperstring
	UserID           whereHelperstring
	InvitationEmail  whereHelpernull_String
	InvitationSentAt whereHelpernull_Time
	CreatedAt        whereHelpernull_Time
	UpdatedAt        whereHelpernull_Time
	CreatedUserID    whereHelpernull_String
}{
	ID:               whereHelperstring{field: "\"user_invitations\".\"id\""},
	UserID:           whereHelperstring{field: "\"user_invitations\".\"user_id\""},
	InvitationEmail:  whereHelpernull_String{field: "\"user_invitations\".\"invitation_email\""},
	InvitationSentAt: whereHelpernull_Time{field: "\"user_invitations\".\"invitation_sent_at\""},
	CreatedAt:        whereHelpernull_Time{field: "\"user_invitations\".\"created_at\""},
	UpdatedAt:        whereHelpernull_Time{field: "\"user_invitations\".\"updated_at\""},
	CreatedUserID:    whereHelpernull_String{field: "\"user_invitations\".\"created_user_id\""},
}

// UserInvitationRels is where relationship names are stored.
var UserInvitationRels = struct {
	CreatedUser string
	User        string
}{
	CreatedUser: "CreatedUser",
	User:        "User",
}

// userInvitationR is where relationships are stored.
type userInvitationR struct {
	CreatedUser *User `boil:"CreatedUser" json:"CreatedUser" toml:"CreatedUser" yaml:"CreatedUser"`
	User        *User `boil:"User" json:"User" toml:"User" yaml:"User"`
}

// NewStruct creates a new relationship struct
func (*userInvitationR) NewStruct() *userInvitationR {
	return &userInvitationR{}
}

func (r *userInvitationR) GetCreatedUser() *User {
	if r == nil {
		return nil
	}
	return r.CreatedUser
}

func (r *userInvitationR) GetUser() *User {
	if r == nil {
		return nil
	}
	return r.User
}

// userInvitationL is where Load methods for each relationship are stored.
type userInvitationL struct{}

var (
	userInvitationAllColumns            = []string{"id", "user_id", "invitation_email", "invitation_sent_at", "created_at", "updated_at", "created_user_id"}
	userInvitationColumnsWithoutDefault = []string{"id", "user_id"}
	userInvitationColumnsWithDefault    = []string{"invitation_email", "invitation_sent_at", "created_at", "updated_at", "created_user_id"}
	userInvitationPrimaryKeyColumns     = []string{"id"}
	userInvitationGeneratedColumns      = []string{}
)

type (
	// UserInvitationSlice is an alias for a slice of pointers to UserInvitation.
	// This should almost always be used instead of []UserInvitation.
	UserInvitationSlice []*UserInvitation

	userInvitationQuery struct {
		*queries.Query
	}
)

// Cache for insert, update and upsert
var (
	userInvitationType                 = reflect.TypeOf(&UserInvitation{})
	userInvitationMapping              = queries.MakeStructMapping(userInvitationType)
	userInvitationPrimaryKeyMapping, _ = queries.BindMapping(userInvitationType, userInvitationMapping, userInvitationPrimaryKeyColumns)
	userInvitationInsertCacheMut       sync.RWMutex
	userInvitationInsertCache          = make(map[string]insertCache)
	userInvitationUpdateCacheMut       sync.RWMutex
	userInvitationUpdateCache          = make(map[string]updateCache)
	userInvitationUpsertCacheMut       sync.RWMutex
	userInvitationUpsertCache          = make(map[string]insertCache)
)

var (
	// Force time package dependency for automated UpdatedAt/CreatedAt.
	_ = time.Second
	// Force qmhelper dependency for where clause generation (which doesn't
	// always happen)
	_ = qmhelper.Where
)

// OneP returns a single userInvitation record from the query, and panics on error.
func (q userInvitationQuery) OneP(ctx context.Context, exec boil.ContextExecutor) *UserInvitation {
	o, err := q.One(ctx, exec)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return o
}

// One returns a single userInvitation record from the query.
func (q userInvitationQuery) One(ctx context.Context, exec boil.ContextExecutor) (*UserInvitation, error) {
	o := &UserInvitation{}

	queries.SetLimit(q.Query, 1)

	err := q.Bind(ctx, exec, o)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, errors.Wrap(err, "core: failed to execute a one query for user_invitations")
	}

	return o, nil
}

// AllP returns all UserInvitation records from the query, and panics on error.
func (q userInvitationQuery) AllP(ctx context.Context, exec boil.ContextExecutor) UserInvitationSlice {
	o, err := q.All(ctx, exec)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return o
}

// All returns all UserInvitation records from the query.
func (q userInvitationQuery) All(ctx context.Context, exec boil.ContextExecutor) (UserInvitationSlice, error) {
	var o []*UserInvitation

	err := q.Bind(ctx, exec, &o)
	if err != nil {
		return nil, errors.Wrap(err, "core: failed to assign all query results to UserInvitation slice")
	}

	return o, nil
}

// CountP returns the count of all UserInvitation records in the query, and panics on error.
func (q userInvitationQuery) CountP(ctx context.Context, exec boil.ContextExecutor) int64 {
	c, err := q.Count(ctx, exec)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return c
}

// Count returns the count of all UserInvitation records in the query.
func (q userInvitationQuery) Count(ctx context.Context, exec boil.ContextExecutor) (int64, error) {
	var count int64

	queries.SetSelect(q.Query, nil)
	queries.SetCount(q.Query)

	err := q.Query.QueryRowContext(ctx, exec).Scan(&count)
	if err != nil {
		return 0, errors.Wrap(err, "core: failed to count user_invitations rows")
	}

	return count, nil
}

// ExistsP checks if the row exists in the table, and panics on error.
func (q userInvitationQuery) ExistsP(ctx context.Context, exec boil.ContextExecutor) bool {
	e, err := q.Exists(ctx, exec)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return e
}

// Exists checks if the row exists in the table.
func (q userInvitationQuery) Exists(ctx context.Context, exec boil.ContextExecutor) (bool, error) {
	var count int64

	queries.SetSelect(q.Query, nil)
	queries.SetCount(q.Query)
	queries.SetLimit(q.Query, 1)

	err := q.Query.QueryRowContext(ctx, exec).Scan(&count)
	if err != nil {
		return false, errors.Wrap(err, "core: failed to check if user_invitations exists")
	}

	return count > 0, nil
}

// CreatedUser pointed to by the foreign key.
func (o *UserInvitation) CreatedUser(mods ...qm.QueryMod) userQuery {
	queryMods := []qm.QueryMod{
		qm.Where("\"id\" = ?", o.CreatedUserID),
	}

	queryMods = append(queryMods, mods...)

	return Users(queryMods...)
}

// User pointed to by the foreign key.
func (o *UserInvitation) User(mods ...qm.QueryMod) userQuery {
	queryMods := []qm.QueryMod{
		qm.Where("\"id\" = ?", o.UserID),
	}

	queryMods = append(queryMods, mods...)

	return Users(queryMods...)
}

// LoadCreatedUser allows an eager lookup of values, cached into the
// loaded structs of the objects. This is for an N-1 relationship.
func (userInvitationL) LoadCreatedUser(ctx context.Context, e boil.ContextExecutor, singular bool, maybeUserInvitation interface{}, mods queries.Applicator) error {
	var slice []*UserInvitation
	var object *UserInvitation

	if singular {
		var ok bool
		object, ok = maybeUserInvitation.(*UserInvitation)
		if !ok {
			object = new(UserInvitation)
			ok = queries.SetFromEmbeddedStruct(&object, &maybeUserInvitation)
			if !ok {
				return errors.New(fmt.Sprintf("failed to set %T from embedded struct %T", object, maybeUserInvitation))
			}
		}
	} else {
		s, ok := maybeUserInvitation.(*[]*UserInvitation)
		if ok {
			slice = *s
		} else {
			ok = queries.SetFromEmbeddedStruct(&slice, maybeUserInvitation)
			if !ok {
				return errors.New(fmt.Sprintf("failed to set %T from embedded struct %T", slice, maybeUserInvitation))
			}
		}
	}

	args := make(map[interface{}]struct{})
	if singular {
		if object.R == nil {
			object.R = &userInvitationR{}
		}
		if !queries.IsNil(object.CreatedUserID) {
			args[object.CreatedUserID] = struct{}{}
		}

	} else {
		for _, obj := range slice {
			if obj.R == nil {
				obj.R = &userInvitationR{}
			}

			if !queries.IsNil(obj.CreatedUserID) {
				args[obj.CreatedUserID] = struct{}{}
			}

		}
	}

	if len(args) == 0 {
		return nil
	}

	argsSlice := make([]interface{}, len(args))
	i := 0
	for arg := range args {
		argsSlice[i] = arg
		i++
	}

	query := NewQuery(
		qm.From(`users`),
		qm.WhereIn(`users.id in ?`, argsSlice...),
	)
	if mods != nil {
		mods.Apply(query)
	}

	results, err := query.QueryContext(ctx, e)
	if err != nil {
		return errors.Wrap(err, "failed to eager load User")
	}

	var resultSlice []*User
	if err = queries.Bind(results, &resultSlice); err != nil {
		return errors.Wrap(err, "failed to bind eager loaded slice User")
	}

	if err = results.Close(); err != nil {
		return errors.Wrap(err, "failed to close results of eager load for users")
	}
	if err = results.Err(); err != nil {
		return errors.Wrap(err, "error occurred during iteration of eager loaded relations for users")
	}

	if len(resultSlice) == 0 {
		return nil
	}

	if singular {
		foreign := resultSlice[0]
		object.R.CreatedUser = foreign
		if foreign.R == nil {
			foreign.R = &userR{}
		}
		foreign.R.CreatedUserUserInvitations = append(foreign.R.CreatedUserUserInvitations, object)
		return nil
	}

	for _, local := range slice {
		for _, foreign := range resultSlice {
			if queries.Equal(local.CreatedUserID, foreign.ID) {
				local.R.CreatedUser = foreign
				if foreign.R == nil {
					foreign.R = &userR{}
				}
				foreign.R.CreatedUserUserInvitations = append(foreign.R.CreatedUserUserInvitations, local)
				break
			}
		}
	}

	return nil
}

// LoadUser allows an eager lookup of values, cached into the
// loaded structs of the objects. This is for an N-1 relationship.
func (userInvitationL) LoadUser(ctx context.Context, e boil.ContextExecutor, singular bool, maybeUserInvitation interface{}, mods queries.Applicator) error {
	var slice []*UserInvitation
	var object *UserInvitation

	if singular {
		var ok bool
		object, ok = maybeUserInvitation.(*UserInvitation)
		if !ok {
			object = new(UserInvitation)
			ok = queries.SetFromEmbeddedStruct(&object, &maybeUserInvitation)
			if !ok {
				return errors.New(fmt.Sprintf("failed to set %T from embedded struct %T", object, maybeUserInvitation))
			}
		}
	} else {
		s, ok := maybeUserInvitation.(*[]*UserInvitation)
		if ok {
			slice = *s
		} else {
			ok = queries.SetFromEmbeddedStruct(&slice, maybeUserInvitation)
			if !ok {
				return errors.New(fmt.Sprintf("failed to set %T from embedded struct %T", slice, maybeUserInvitation))
			}
		}
	}

	args := make(map[interface{}]struct{})
	if singular {
		if object.R == nil {
			object.R = &userInvitationR{}
		}
		args[object.UserID] = struct{}{}

	} else {
		for _, obj := range slice {
			if obj.R == nil {
				obj.R = &userInvitationR{}
			}

			args[obj.UserID] = struct{}{}

		}
	}

	if len(args) == 0 {
		return nil
	}

	argsSlice := make([]interface{}, len(args))
	i := 0
	for arg := range args {
		argsSlice[i] = arg
		i++
	}

	query := NewQuery(
		qm.From(`users`),
		qm.WhereIn(`users.id in ?`, argsSlice...),
	)
	if mods != nil {
		mods.Apply(query)
	}

	results, err := query.QueryContext(ctx, e)
	if err != nil {
		return errors.Wrap(err, "failed to eager load User")
	}

	var resultSlice []*User
	if err = queries.Bind(results, &resultSlice); err != nil {
		return errors.Wrap(err, "failed to bind eager loaded slice User")
	}

	if err = results.Close(); err != nil {
		return errors.Wrap(err, "failed to close results of eager load for users")
	}
	if err = results.Err(); err != nil {
		return errors.Wrap(err, "error occurred during iteration of eager loaded relations for users")
	}

	if len(resultSlice) == 0 {
		return nil
	}

	if singular {
		foreign := resultSlice[0]
		object.R.User = foreign
		if foreign.R == nil {
			foreign.R = &userR{}
		}
		foreign.R.UserInvitations = append(foreign.R.UserInvitations, object)
		return nil
	}

	for _, local := range slice {
		for _, foreign := range resultSlice {
			if local.UserID == foreign.ID {
				local.R.User = foreign
				if foreign.R == nil {
					foreign.R = &userR{}
				}
				foreign.R.UserInvitations = append(foreign.R.UserInvitations, local)
				break
			}
		}
	}

	return nil
}

// SetCreatedUserP of the userInvitation to the related item.
// Sets o.R.CreatedUser to related.
// Adds o to related.R.CreatedUserUserInvitations.
// Panics on error.
func (o *UserInvitation) SetCreatedUserP(ctx context.Context, exec boil.ContextExecutor, insert bool, related *User) {
	if err := o.SetCreatedUser(ctx, exec, insert, related); err != nil {
		panic(boil.WrapErr(err))
	}
}

// SetCreatedUser of the userInvitation to the related item.
// Sets o.R.CreatedUser to related.
// Adds o to related.R.CreatedUserUserInvitations.
func (o *UserInvitation) SetCreatedUser(ctx context.Context, exec boil.ContextExecutor, insert bool, related *User) error {
	var err error
	if insert {
		if err = related.Insert(ctx, exec, boil.Infer()); err != nil {
			return errors.Wrap(err, "failed to insert into foreign table")
		}
	}

	updateQuery := fmt.Sprintf(
		"UPDATE \"user_invitations\" SET %s WHERE %s",
		strmangle.SetParamNames("\"", "\"", 1, []string{"created_user_id"}),
		strmangle.WhereClause("\"", "\"", 2, userInvitationPrimaryKeyColumns),
	)
	values := []interface{}{related.ID, o.ID}

	if boil.IsDebug(ctx) {
		writer := boil.DebugWriterFrom(ctx)
		fmt.Fprintln(writer, updateQuery)
		fmt.Fprintln(writer, values)
	}
	if _, err = exec.ExecContext(ctx, updateQuery, values...); err != nil {
		return errors.Wrap(err, "failed to update local table")
	}

	queries.Assign(&o.CreatedUserID, related.ID)
	if o.R == nil {
		o.R = &userInvitationR{
			CreatedUser: related,
		}
	} else {
		o.R.CreatedUser = related
	}

	if related.R == nil {
		related.R = &userR{
			CreatedUserUserInvitations: UserInvitationSlice{o},
		}
	} else {
		related.R.CreatedUserUserInvitations = append(related.R.CreatedUserUserInvitations, o)
	}

	return nil
}

// RemoveCreatedUserP relationship.
// Sets o.R.CreatedUser to nil.
// Removes o from all passed in related items' relationships struct.
// Panics on error.
func (o *UserInvitation) RemoveCreatedUserP(ctx context.Context, exec boil.ContextExecutor, related *User) {
	if err := o.RemoveCreatedUser(ctx, exec, related); err != nil {
		panic(boil.WrapErr(err))
	}
}

// RemoveCreatedUser relationship.
// Sets o.R.CreatedUser to nil.
// Removes o from all passed in related items' relationships struct.
func (o *UserInvitation) RemoveCreatedUser(ctx context.Context, exec boil.ContextExecutor, related *User) error {
	var err error

	queries.SetScanner(&o.CreatedUserID, nil)
	if _, err = o.Update(ctx, exec, boil.Whitelist("created_user_id")); err != nil {
		return errors.Wrap(err, "failed to update local table")
	}

	if o.R != nil {
		o.R.CreatedUser = nil
	}
	if related == nil || related.R == nil {
		return nil
	}

	for i, ri := range related.R.CreatedUserUserInvitations {
		if queries.Equal(o.CreatedUserID, ri.CreatedUserID) {
			continue
		}

		ln := len(related.R.CreatedUserUserInvitations)
		if ln > 1 && i < ln-1 {
			related.R.CreatedUserUserInvitations[i] = related.R.CreatedUserUserInvitations[ln-1]
		}
		related.R.CreatedUserUserInvitations = related.R.CreatedUserUserInvitations[:ln-1]
		break
	}
	return nil
}

// SetUserP of the userInvitation to the related item.
// Sets o.R.User to related.
// Adds o to related.R.UserInvitations.
// Panics on error.
func (o *UserInvitation) SetUserP(ctx context.Context, exec boil.ContextExecutor, insert bool, related *User) {
	if err := o.SetUser(ctx, exec, insert, related); err != nil {
		panic(boil.WrapErr(err))
	}
}

// SetUser of the userInvitation to the related item.
// Sets o.R.User to related.
// Adds o to related.R.UserInvitations.
func (o *UserInvitation) SetUser(ctx context.Context, exec boil.ContextExecutor, insert bool, related *User) error {
	var err error
	if insert {
		if err = related.Insert(ctx, exec, boil.Infer()); err != nil {
			return errors.Wrap(err, "failed to insert into foreign table")
		}
	}

	updateQuery := fmt.Sprintf(
		"UPDATE \"user_invitations\" SET %s WHERE %s",
		strmangle.SetParamNames("\"", "\"", 1, []string{"user_id"}),
		strmangle.WhereClause("\"", "\"", 2, userInvitationPrimaryKeyColumns),
	)
	values := []interface{}{related.ID, o.ID}

	if boil.IsDebug(ctx) {
		writer := boil.DebugWriterFrom(ctx)
		fmt.Fprintln(writer, updateQuery)
		fmt.Fprintln(writer, values)
	}
	if _, err = exec.ExecContext(ctx, updateQuery, values...); err != nil {
		return errors.Wrap(err, "failed to update local table")
	}

	o.UserID = related.ID
	if o.R == nil {
		o.R = &userInvitationR{
			User: related,
		}
	} else {
		o.R.User = related
	}

	if related.R == nil {
		related.R = &userR{
			UserInvitations: UserInvitationSlice{o},
		}
	} else {
		related.R.UserInvitations = append(related.R.UserInvitations, o)
	}

	return nil
}

// UserInvitations retrieves all the records using an executor.
func UserInvitations(mods ...qm.QueryMod) userInvitationQuery {
	mods = append(mods, qm.From("\"user_invitations\""))
	q := NewQuery(mods...)
	if len(queries.GetSelect(q)) == 0 {
		queries.SetSelect(q, []string{"\"user_invitations\".*"})
	}

	return userInvitationQuery{q}
}

// FindUserInvitationP retrieves a single record by ID with an executor, and panics on error.
func FindUserInvitationP(ctx context.Context, exec boil.ContextExecutor, iD string, selectCols ...string) *UserInvitation {
	retobj, err := FindUserInvitation(ctx, exec, iD, selectCols...)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return retobj
}

// FindUserInvitation retrieves a single record by ID with an executor.
// If selectCols is empty Find will return all columns.
func FindUserInvitation(ctx context.Context, exec boil.ContextExecutor, iD string, selectCols ...string) (*UserInvitation, error) {
	userInvitationObj := &UserInvitation{}

	sel := "*"
	if len(selectCols) > 0 {
		sel = strings.Join(strmangle.IdentQuoteSlice(dialect.LQ, dialect.RQ, selectCols), ",")
	}
	query := fmt.Sprintf(
		"select %s from \"user_invitations\" where \"id\"=$1", sel,
	)

	q := queries.Raw(query, iD)

	err := q.Bind(ctx, exec, userInvitationObj)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, errors.Wrap(err, "core: unable to select from user_invitations")
	}

	return userInvitationObj, nil
}

// InsertP a single record using an executor, and panics on error. See Insert
// for whitelist behavior description.
func (o *UserInvitation) InsertP(ctx context.Context, exec boil.ContextExecutor, columns boil.Columns) {
	if err := o.Insert(ctx, exec, columns); err != nil {
		panic(boil.WrapErr(err))
	}
}

// Insert a single record using an executor.
// See boil.Columns.InsertColumnSet documentation to understand column list inference for inserts.
func (o *UserInvitation) Insert(ctx context.Context, exec boil.ContextExecutor, columns boil.Columns) error {
	if o == nil {
		return errors.New("core: no user_invitations provided for insertion")
	}

	var err error
	if !boil.TimestampsAreSkipped(ctx) {
		currTime := time.Now().In(boil.GetLocation())

		if queries.MustTime(o.CreatedAt).IsZero() {
			queries.SetScanner(&o.CreatedAt, currTime)
		}
		if queries.MustTime(o.UpdatedAt).IsZero() {
			queries.SetScanner(&o.UpdatedAt, currTime)
		}
	}

	nzDefaults := queries.NonZeroDefaultSet(userInvitationColumnsWithDefault, o)

	key := makeCacheKey(columns, nzDefaults)
	userInvitationInsertCacheMut.RLock()
	cache, cached := userInvitationInsertCache[key]
	userInvitationInsertCacheMut.RUnlock()

	if !cached {
		wl, returnColumns := columns.InsertColumnSet(
			userInvitationAllColumns,
			userInvitationColumnsWithDefault,
			userInvitationColumnsWithoutDefault,
			nzDefaults,
		)

		cache.valueMapping, err = queries.BindMapping(userInvitationType, userInvitationMapping, wl)
		if err != nil {
			return err
		}
		cache.retMapping, err = queries.BindMapping(userInvitationType, userInvitationMapping, returnColumns)
		if err != nil {
			return err
		}
		if len(wl) != 0 {
			cache.query = fmt.Sprintf("INSERT INTO \"user_invitations\" (\"%s\") %%sVALUES (%s)%%s", strings.Join(wl, "\",\""), strmangle.Placeholders(dialect.UseIndexPlaceholders, len(wl), 1, 1))
		} else {
			cache.query = "INSERT INTO \"user_invitations\" %sDEFAULT VALUES%s"
		}

		var queryOutput, queryReturning string

		if len(cache.retMapping) != 0 {
			queryReturning = fmt.Sprintf(" RETURNING \"%s\"", strings.Join(returnColumns, "\",\""))
		}

		cache.query = fmt.Sprintf(cache.query, queryOutput, queryReturning)
	}

	value := reflect.Indirect(reflect.ValueOf(o))
	vals := queries.ValuesFromMapping(value, cache.valueMapping)

	if boil.IsDebug(ctx) {
		writer := boil.DebugWriterFrom(ctx)
		fmt.Fprintln(writer, cache.query)
		fmt.Fprintln(writer, vals)
	}

	if len(cache.retMapping) != 0 {
		err = exec.QueryRowContext(ctx, cache.query, vals...).Scan(queries.PtrsFromMapping(value, cache.retMapping)...)
	} else {
		_, err = exec.ExecContext(ctx, cache.query, vals...)
	}

	if err != nil {
		return errors.Wrap(err, "core: unable to insert into user_invitations")
	}

	if !cached {
		userInvitationInsertCacheMut.Lock()
		userInvitationInsertCache[key] = cache
		userInvitationInsertCacheMut.Unlock()
	}

	return nil
}

// UpdateP uses an executor to update the UserInvitation, and panics on error.
// See Update for more documentation.
func (o *UserInvitation) UpdateP(ctx context.Context, exec boil.ContextExecutor, columns boil.Columns) int64 {
	rowsAff, err := o.Update(ctx, exec, columns)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return rowsAff
}

// Update uses an executor to update the UserInvitation.
// See boil.Columns.UpdateColumnSet documentation to understand column list inference for updates.
// Update does not automatically update the record in case of default values. Use .Reload() to refresh the records.
func (o *UserInvitation) Update(ctx context.Context, exec boil.ContextExecutor, columns boil.Columns) (int64, error) {
	if !boil.TimestampsAreSkipped(ctx) {
		currTime := time.Now().In(boil.GetLocation())

		queries.SetScanner(&o.UpdatedAt, currTime)
	}

	var err error
	key := makeCacheKey(columns, nil)
	userInvitationUpdateCacheMut.RLock()
	cache, cached := userInvitationUpdateCache[key]
	userInvitationUpdateCacheMut.RUnlock()

	if !cached {
		wl := columns.UpdateColumnSet(
			userInvitationAllColumns,
			userInvitationPrimaryKeyColumns,
		)

		if !columns.IsWhitelist() {
			wl = strmangle.SetComplement(wl, []string{"created_at"})
		}
		if len(wl) == 0 {
			return 0, errors.New("core: unable to update user_invitations, could not build whitelist")
		}

		cache.query = fmt.Sprintf("UPDATE \"user_invitations\" SET %s WHERE %s",
			strmangle.SetParamNames("\"", "\"", 1, wl),
			strmangle.WhereClause("\"", "\"", len(wl)+1, userInvitationPrimaryKeyColumns),
		)
		cache.valueMapping, err = queries.BindMapping(userInvitationType, userInvitationMapping, append(wl, userInvitationPrimaryKeyColumns...))
		if err != nil {
			return 0, err
		}
	}

	values := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(o)), cache.valueMapping)

	if boil.IsDebug(ctx) {
		writer := boil.DebugWriterFrom(ctx)
		fmt.Fprintln(writer, cache.query)
		fmt.Fprintln(writer, values)
	}
	var result sql.Result
	result, err = exec.ExecContext(ctx, cache.query, values...)
	if err != nil {
		return 0, errors.Wrap(err, "core: unable to update user_invitations row")
	}

	rowsAff, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, "core: failed to get rows affected by update for user_invitations")
	}

	if !cached {
		userInvitationUpdateCacheMut.Lock()
		userInvitationUpdateCache[key] = cache
		userInvitationUpdateCacheMut.Unlock()
	}

	return rowsAff, nil
}

// UpdateAllP updates all rows with matching column names, and panics on error.
func (q userInvitationQuery) UpdateAllP(ctx context.Context, exec boil.ContextExecutor, cols M) int64 {
	rowsAff, err := q.UpdateAll(ctx, exec, cols)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return rowsAff
}

// UpdateAll updates all rows with the specified column values.
func (q userInvitationQuery) UpdateAll(ctx context.Context, exec boil.ContextExecutor, cols M) (int64, error) {
	queries.SetUpdate(q.Query, cols)

	result, err := q.Query.ExecContext(ctx, exec)
	if err != nil {
		return 0, errors.Wrap(err, "core: unable to update all for user_invitations")
	}

	rowsAff, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, "core: unable to retrieve rows affected for user_invitations")
	}

	return rowsAff, nil
}

// UpdateAllP updates all rows with the specified column values, and panics on error.
func (o UserInvitationSlice) UpdateAllP(ctx context.Context, exec boil.ContextExecutor, cols M) int64 {
	rowsAff, err := o.UpdateAll(ctx, exec, cols)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return rowsAff
}

// UpdateAll updates all rows with the specified column values, using an executor.
func (o UserInvitationSlice) UpdateAll(ctx context.Context, exec boil.ContextExecutor, cols M) (int64, error) {
	ln := int64(len(o))
	if ln == 0 {
		return 0, nil
	}

	if len(cols) == 0 {
		return 0, errors.New("core: update all requires at least one column argument")
	}

	colNames := make([]string, len(cols))
	args := make([]interface{}, len(cols))

	i := 0
	for name, value := range cols {
		colNames[i] = name
		args[i] = value
		i++
	}

	// Append all of the primary key values for each column
	for _, obj := range o {
		pkeyArgs := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(obj)), userInvitationPrimaryKeyMapping)
		args = append(args, pkeyArgs...)
	}

	sql := fmt.Sprintf("UPDATE \"user_invitations\" SET %s WHERE %s",
		strmangle.SetParamNames("\"", "\"", 1, colNames),
		strmangle.WhereClauseRepeated(string(dialect.LQ), string(dialect.RQ), len(colNames)+1, userInvitationPrimaryKeyColumns, len(o)))

	if boil.IsDebug(ctx) {
		writer := boil.DebugWriterFrom(ctx)
		fmt.Fprintln(writer, sql)
		fmt.Fprintln(writer, args...)
	}
	result, err := exec.ExecContext(ctx, sql, args...)
	if err != nil {
		return 0, errors.Wrap(err, "core: unable to update all in userInvitation slice")
	}

	rowsAff, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, "core: unable to retrieve rows affected all in update all userInvitation")
	}
	return rowsAff, nil
}

// UpsertP attempts an insert using an executor, and does an update or ignore on conflict.
// UpsertP panics on error.
func (o *UserInvitation) UpsertP(ctx context.Context, exec boil.ContextExecutor, updateOnConflict bool, conflictColumns []string, updateColumns, insertColumns boil.Columns, opts ...UpsertOptionFunc) {
	if err := o.Upsert(ctx, exec, updateOnConflict, conflictColumns, updateColumns, insertColumns, opts...); err != nil {
		panic(boil.WrapErr(err))
	}
}

// Upsert attempts an insert using an executor, and does an update or ignore on conflict.
// See boil.Columns documentation for how to properly use updateColumns and insertColumns.
func (o *UserInvitation) Upsert(ctx context.Context, exec boil.ContextExecutor, updateOnConflict bool, conflictColumns []string, updateColumns, insertColumns boil.Columns, opts ...UpsertOptionFunc) error {
	if o == nil {
		return errors.New("core: no user_invitations provided for upsert")
	}
	if !boil.TimestampsAreSkipped(ctx) {
		currTime := time.Now().In(boil.GetLocation())

		if queries.MustTime(o.CreatedAt).IsZero() {
			queries.SetScanner(&o.CreatedAt, currTime)
		}
		queries.SetScanner(&o.UpdatedAt, currTime)
	}

	nzDefaults := queries.NonZeroDefaultSet(userInvitationColumnsWithDefault, o)

	// Build cache key in-line uglily - mysql vs psql problems
	buf := strmangle.GetBuffer()
	if updateOnConflict {
		buf.WriteByte('t')
	} else {
		buf.WriteByte('f')
	}
	buf.WriteByte('.')
	for _, c := range conflictColumns {
		buf.WriteString(c)
	}
	buf.WriteByte('.')
	buf.WriteString(strconv.Itoa(updateColumns.Kind))
	for _, c := range updateColumns.Cols {
		buf.WriteString(c)
	}
	buf.WriteByte('.')
	buf.WriteString(strconv.Itoa(insertColumns.Kind))
	for _, c := range insertColumns.Cols {
		buf.WriteString(c)
	}
	buf.WriteByte('.')
	for _, c := range nzDefaults {
		buf.WriteString(c)
	}
	key := buf.String()
	strmangle.PutBuffer(buf)

	userInvitationUpsertCacheMut.RLock()
	cache, cached := userInvitationUpsertCache[key]
	userInvitationUpsertCacheMut.RUnlock()

	var err error

	if !cached {
		insert, _ := insertColumns.InsertColumnSet(
			userInvitationAllColumns,
			userInvitationColumnsWithDefault,
			userInvitationColumnsWithoutDefault,
			nzDefaults,
		)

		update := updateColumns.UpdateColumnSet(
			userInvitationAllColumns,
			userInvitationPrimaryKeyColumns,
		)

		if updateOnConflict && len(update) == 0 {
			return errors.New("core: unable to upsert user_invitations, could not build update column list")
		}

		ret := strmangle.SetComplement(userInvitationAllColumns, strmangle.SetIntersect(insert, update))

		conflict := conflictColumns
		if len(conflict) == 0 && updateOnConflict && len(update) != 0 {
			if len(userInvitationPrimaryKeyColumns) == 0 {
				return errors.New("core: unable to upsert user_invitations, could not build conflict column list")
			}

			conflict = make([]string, len(userInvitationPrimaryKeyColumns))
			copy(conflict, userInvitationPrimaryKeyColumns)
		}
		cache.query = buildUpsertQueryPostgres(dialect, "\"user_invitations\"", updateOnConflict, ret, update, conflict, insert, opts...)

		cache.valueMapping, err = queries.BindMapping(userInvitationType, userInvitationMapping, insert)
		if err != nil {
			return err
		}
		if len(ret) != 0 {
			cache.retMapping, err = queries.BindMapping(userInvitationType, userInvitationMapping, ret)
			if err != nil {
				return err
			}
		}
	}

	value := reflect.Indirect(reflect.ValueOf(o))
	vals := queries.ValuesFromMapping(value, cache.valueMapping)
	var returns []interface{}
	if len(cache.retMapping) != 0 {
		returns = queries.PtrsFromMapping(value, cache.retMapping)
	}

	if boil.IsDebug(ctx) {
		writer := boil.DebugWriterFrom(ctx)
		fmt.Fprintln(writer, cache.query)
		fmt.Fprintln(writer, vals)
	}
	if len(cache.retMapping) != 0 {
		err = exec.QueryRowContext(ctx, cache.query, vals...).Scan(returns...)
		if errors.Is(err, sql.ErrNoRows) {
			err = nil // Postgres doesn't return anything when there's no update
		}
	} else {
		_, err = exec.ExecContext(ctx, cache.query, vals...)
	}
	if err != nil {
		return errors.Wrap(err, "core: unable to upsert user_invitations")
	}

	if !cached {
		userInvitationUpsertCacheMut.Lock()
		userInvitationUpsertCache[key] = cache
		userInvitationUpsertCacheMut.Unlock()
	}

	return nil
}

// DeleteP deletes a single UserInvitation record with an executor.
// DeleteP will match against the primary key column to find the record to delete.
// Panics on error.
func (o *UserInvitation) DeleteP(ctx context.Context, exec boil.ContextExecutor) int64 {
	rowsAff, err := o.Delete(ctx, exec)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return rowsAff
}

// Delete deletes a single UserInvitation record with an executor.
// Delete will match against the primary key column to find the record to delete.
func (o *UserInvitation) Delete(ctx context.Context, exec boil.ContextExecutor) (int64, error) {
	if o == nil {
		return 0, errors.New("core: no UserInvitation provided for delete")
	}

	args := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(o)), userInvitationPrimaryKeyMapping)
	sql := "DELETE FROM \"user_invitations\" WHERE \"id\"=$1"

	if boil.IsDebug(ctx) {
		writer := boil.DebugWriterFrom(ctx)
		fmt.Fprintln(writer, sql)
		fmt.Fprintln(writer, args...)
	}
	result, err := exec.ExecContext(ctx, sql, args...)
	if err != nil {
		return 0, errors.Wrap(err, "core: unable to delete from user_invitations")
	}

	rowsAff, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, "core: failed to get rows affected by delete for user_invitations")
	}

	return rowsAff, nil
}

// DeleteAllP deletes all rows, and panics on error.
func (q userInvitationQuery) DeleteAllP(ctx context.Context, exec boil.ContextExecutor) int64 {
	rowsAff, err := q.DeleteAll(ctx, exec)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return rowsAff
}

// DeleteAll deletes all matching rows.
func (q userInvitationQuery) DeleteAll(ctx context.Context, exec boil.ContextExecutor) (int64, error) {
	if q.Query == nil {
		return 0, errors.New("core: no userInvitationQuery provided for delete all")
	}

	queries.SetDelete(q.Query)

	result, err := q.Query.ExecContext(ctx, exec)
	if err != nil {
		return 0, errors.Wrap(err, "core: unable to delete all from user_invitations")
	}

	rowsAff, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, "core: failed to get rows affected by deleteall for user_invitations")
	}

	return rowsAff, nil
}

// DeleteAllP deletes all rows in the slice, using an executor, and panics on error.
func (o UserInvitationSlice) DeleteAllP(ctx context.Context, exec boil.ContextExecutor) int64 {
	rowsAff, err := o.DeleteAll(ctx, exec)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return rowsAff
}

// DeleteAll deletes all rows in the slice, using an executor.
func (o UserInvitationSlice) DeleteAll(ctx context.Context, exec boil.ContextExecutor) (int64, error) {
	if len(o) == 0 {
		return 0, nil
	}

	var args []interface{}
	for _, obj := range o {
		pkeyArgs := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(obj)), userInvitationPrimaryKeyMapping)
		args = append(args, pkeyArgs...)
	}

	sql := "DELETE FROM \"user_invitations\" WHERE " +
		strmangle.WhereClauseRepeated(string(dialect.LQ), string(dialect.RQ), 1, userInvitationPrimaryKeyColumns, len(o))

	if boil.IsDebug(ctx) {
		writer := boil.DebugWriterFrom(ctx)
		fmt.Fprintln(writer, sql)
		fmt.Fprintln(writer, args)
	}
	result, err := exec.ExecContext(ctx, sql, args...)
	if err != nil {
		return 0, errors.Wrap(err, "core: unable to delete all from userInvitation slice")
	}

	rowsAff, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, "core: failed to get rows affected by deleteall for user_invitations")
	}

	return rowsAff, nil
}

// ReloadP refetches the object from the database with an executor. Panics on error.
func (o *UserInvitation) ReloadP(ctx context.Context, exec boil.ContextExecutor) {
	if err := o.Reload(ctx, exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// Reload refetches the object from the database
// using the primary keys with an executor.
func (o *UserInvitation) Reload(ctx context.Context, exec boil.ContextExecutor) error {
	ret, err := FindUserInvitation(ctx, exec, o.ID)
	if err != nil {
		return err
	}

	*o = *ret
	return nil
}

// ReloadAllP refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
// Panics on error.
func (o *UserInvitationSlice) ReloadAllP(ctx context.Context, exec boil.ContextExecutor) {
	if err := o.ReloadAll(ctx, exec); err != nil {
		panic(boil.WrapErr(err))
	}
}

// ReloadAll refetches every row with matching primary key column values
// and overwrites the original object slice with the newly updated slice.
func (o *UserInvitationSlice) ReloadAll(ctx context.Context, exec boil.ContextExecutor) error {
	if o == nil || len(*o) == 0 {
		return nil
	}

	slice := UserInvitationSlice{}
	var args []interface{}
	for _, obj := range *o {
		pkeyArgs := queries.ValuesFromMapping(reflect.Indirect(reflect.ValueOf(obj)), userInvitationPrimaryKeyMapping)
		args = append(args, pkeyArgs...)
	}

	sql := "SELECT \"user_invitations\".* FROM \"user_invitations\" WHERE " +
		strmangle.WhereClauseRepeated(string(dialect.LQ), string(dialect.RQ), 1, userInvitationPrimaryKeyColumns, len(*o))

	q := queries.Raw(sql, args...)

	err := q.Bind(ctx, exec, &slice)
	if err != nil {
		return errors.Wrap(err, "core: unable to reload all in UserInvitationSlice")
	}

	*o = slice

	return nil
}

// UserInvitationExistsP checks if the UserInvitation row exists. Panics on error.
func UserInvitationExistsP(ctx context.Context, exec boil.ContextExecutor, iD string) bool {
	e, err := UserInvitationExists(ctx, exec, iD)
	if err != nil {
		panic(boil.WrapErr(err))
	}

	return e
}

// UserInvitationExists checks if the UserInvitation row exists.
func UserInvitationExists(ctx context.Context, exec boil.ContextExecutor, iD string) (bool, error) {
	var exists bool
	sql := "select exists(select 1 from \"user_invitations\" where \"id\"=$1 limit 1)"

	if boil.IsDebug(ctx) {
		writer := boil.DebugWriterFrom(ctx)
		fmt.Fprintln(writer, sql)
		fmt.Fprintln(writer, iD)
	}
	row := exec.QueryRowContext(ctx, sql, iD)

	err := row.Scan(&exists)
	if err != nil {
		return false, errors.Wrap(err, "core: unable to check if user_invitations exists")
	}

	return exists, nil
}

// Exists checks if the UserInvitation row exists.
func (o *UserInvitation) Exists(ctx context.Context, exec boil.ContextExecutor) (bool, error) {
	return UserInvitationExists(ctx, exec, o.ID)
}
