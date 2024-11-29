// Code generated by SQLBoiler 4.16.2 (https://github.com/volatiletech/sqlboiler). DO NOT EDIT.
// This file is meant to be re-generated in place and/or deleted at any time.

package core

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"strconv"

	"github.com/friendsofgo/errors"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/null/v8/convert"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/strmangle"
)

// M type is for providing columns and column values to UpdateAll.
type M map[string]interface{}

// ErrSyncFail occurs during insert when the record could not be retrieved in
// order to populate default value information. This usually happens when LastInsertId
// fails or there was a primary key configuration that was not resolvable.
var ErrSyncFail = errors.New("core: failed to synchronize data after insert")

type insertCache struct {
	query        string
	retQuery     string
	valueMapping []uint64
	retMapping   []uint64
}

type updateCache struct {
	query        string
	valueMapping []uint64
}

func makeCacheKey(cols boil.Columns, nzDefaults []string) string {
	buf := strmangle.GetBuffer()

	buf.WriteString(strconv.Itoa(cols.Kind))
	for _, w := range cols.Cols {
		buf.WriteString(w)
	}

	if len(nzDefaults) != 0 {
		buf.WriteByte('.')
	}
	for _, nz := range nzDefaults {
		buf.WriteString(nz)
	}

	str := buf.String()
	strmangle.PutBuffer(buf)
	return str
}

type OutgoingEmailStatus string

// Enum values for OutgoingEmailStatus
const (
	OutgoingEmailStatusNew    OutgoingEmailStatus = "new"
	OutgoingEmailStatusSent   OutgoingEmailStatus = "sent"
	OutgoingEmailStatusFailed OutgoingEmailStatus = "failed"
)

func AllOutgoingEmailStatus() []OutgoingEmailStatus {
	return []OutgoingEmailStatus{
		OutgoingEmailStatusNew,
		OutgoingEmailStatusSent,
		OutgoingEmailStatusFailed,
	}
}

func (e OutgoingEmailStatus) IsValid() error {
	switch e {
	case OutgoingEmailStatusNew, OutgoingEmailStatusSent, OutgoingEmailStatusFailed:
		return nil
	default:
		return errors.New("enum is not valid")
	}
}

func (e OutgoingEmailStatus) String() string {
	return string(e)
}

func (e OutgoingEmailStatus) Ordinal() int {
	switch e {
	case OutgoingEmailStatusNew:
		return 0
	case OutgoingEmailStatusSent:
		return 1
	case OutgoingEmailStatusFailed:
		return 2

	default:
		panic(errors.New("enum is not valid"))
	}
}

type PostVisibility string

// Enum values for PostVisibility
const (
	PostVisibilityDirectOnly   PostVisibility = "direct_only"
	PostVisibilitySecondDegree PostVisibility = "second_degree"
	PostVisibilityPublic       PostVisibility = "public"
)

func AllPostVisibility() []PostVisibility {
	return []PostVisibility{
		PostVisibilityDirectOnly,
		PostVisibilitySecondDegree,
		PostVisibilityPublic,
	}
}

func (e PostVisibility) IsValid() error {
	switch e {
	case PostVisibilityDirectOnly, PostVisibilitySecondDegree, PostVisibilityPublic:
		return nil
	default:
		return errors.New("enum is not valid")
	}
}

func (e PostVisibility) String() string {
	return string(e)
}

func (e PostVisibility) Ordinal() int {
	switch e {
	case PostVisibilityDirectOnly:
		return 0
	case PostVisibilitySecondDegree:
		return 1
	case PostVisibilityPublic:
		return 2

	default:
		panic(errors.New("enum is not valid"))
	}
}

type ConnectionRequestDecision string

// Enum values for ConnectionRequestDecision
const (
	ConnectionRequestDecisionApproved  ConnectionRequestDecision = "approved"
	ConnectionRequestDecisionDismissed ConnectionRequestDecision = "dismissed"
)

func AllConnectionRequestDecision() []ConnectionRequestDecision {
	return []ConnectionRequestDecision{
		ConnectionRequestDecisionApproved,
		ConnectionRequestDecisionDismissed,
	}
}

func (e ConnectionRequestDecision) IsValid() error {
	switch e {
	case ConnectionRequestDecisionApproved, ConnectionRequestDecisionDismissed:
		return nil
	default:
		return errors.New("enum is not valid")
	}
}

func (e ConnectionRequestDecision) String() string {
	return string(e)
}

func (e ConnectionRequestDecision) Ordinal() int {
	switch e {
	case ConnectionRequestDecisionApproved:
		return 0
	case ConnectionRequestDecisionDismissed:
		return 1

	default:
		panic(errors.New("enum is not valid"))
	}
}

// NullConnectionRequestDecision is a nullable ConnectionRequestDecision enum type. It supports SQL and JSON serialization.
type NullConnectionRequestDecision struct {
	Val   ConnectionRequestDecision
	Valid bool
}

// NullConnectionRequestDecisionFrom creates a new ConnectionRequestDecision that will never be blank.
func NullConnectionRequestDecisionFrom(v ConnectionRequestDecision) NullConnectionRequestDecision {
	return NewNullConnectionRequestDecision(v, true)
}

// NullConnectionRequestDecisionFromPtr creates a new NullConnectionRequestDecision that be null if s is nil.
func NullConnectionRequestDecisionFromPtr(v *ConnectionRequestDecision) NullConnectionRequestDecision {
	if v == nil {
		return NewNullConnectionRequestDecision("", false)
	}
	return NewNullConnectionRequestDecision(*v, true)
}

// NewNullConnectionRequestDecision creates a new NullConnectionRequestDecision
func NewNullConnectionRequestDecision(v ConnectionRequestDecision, valid bool) NullConnectionRequestDecision {
	return NullConnectionRequestDecision{
		Val:   v,
		Valid: valid,
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (e *NullConnectionRequestDecision) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, null.NullBytes) {
		e.Val = ""
		e.Valid = false
		return nil
	}

	if err := json.Unmarshal(data, &e.Val); err != nil {
		return err
	}

	e.Valid = true
	return nil
}

// MarshalJSON implements json.Marshaler.
func (e NullConnectionRequestDecision) MarshalJSON() ([]byte, error) {
	if !e.Valid {
		return null.NullBytes, nil
	}
	return json.Marshal(e.Val)
}

// MarshalText implements encoding.TextMarshaler.
func (e NullConnectionRequestDecision) MarshalText() ([]byte, error) {
	if !e.Valid {
		return []byte{}, nil
	}
	return []byte(e.Val), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (e *NullConnectionRequestDecision) UnmarshalText(text []byte) error {
	if text == nil || len(text) == 0 {
		e.Valid = false
		return nil
	}

	e.Val = ConnectionRequestDecision(text)
	e.Valid = true
	return nil
}

// SetValid changes this NullConnectionRequestDecision value and also sets it to be non-null.
func (e *NullConnectionRequestDecision) SetValid(v ConnectionRequestDecision) {
	e.Val = v
	e.Valid = true
}

// Ptr returns a pointer to this NullConnectionRequestDecision value, or a nil pointer if this NullConnectionRequestDecision is null.
func (e NullConnectionRequestDecision) Ptr() *ConnectionRequestDecision {
	if !e.Valid {
		return nil
	}
	return &e.Val
}

// IsZero returns true for null types.
func (e NullConnectionRequestDecision) IsZero() bool {
	return !e.Valid
}

// Scan implements the Scanner interface.
func (e *NullConnectionRequestDecision) Scan(value interface{}) error {
	if value == nil {
		e.Val, e.Valid = "", false
		return nil
	}
	e.Valid = true
	return convert.ConvertAssign((*string)(&e.Val), value)
}

// Value implements the driver Valuer interface.
func (e NullConnectionRequestDecision) Value() (driver.Value, error) {
	if !e.Valid {
		return nil, nil
	}
	return string(e.Val), nil
}

type ConnectionMediationDecision string

// Enum values for ConnectionMediationDecision
const (
	ConnectionMediationDecisionSigned    ConnectionMediationDecision = "signed"
	ConnectionMediationDecisionDismissed ConnectionMediationDecision = "dismissed"
)

func AllConnectionMediationDecision() []ConnectionMediationDecision {
	return []ConnectionMediationDecision{
		ConnectionMediationDecisionSigned,
		ConnectionMediationDecisionDismissed,
	}
}

func (e ConnectionMediationDecision) IsValid() error {
	switch e {
	case ConnectionMediationDecisionSigned, ConnectionMediationDecisionDismissed:
		return nil
	default:
		return errors.New("enum is not valid")
	}
}

func (e ConnectionMediationDecision) String() string {
	return string(e)
}

func (e ConnectionMediationDecision) Ordinal() int {
	switch e {
	case ConnectionMediationDecisionSigned:
		return 0
	case ConnectionMediationDecisionDismissed:
		return 1

	default:
		panic(errors.New("enum is not valid"))
	}
}

type ProfileVisibility string

// Enum values for ProfileVisibility
const (
	ProfileVisibilityConnections     ProfileVisibility = "connections"
	ProfileVisibilityRegisteredUsers ProfileVisibility = "registered_users"
	ProfileVisibilityPublic          ProfileVisibility = "public"
)

func AllProfileVisibility() []ProfileVisibility {
	return []ProfileVisibility{
		ProfileVisibilityConnections,
		ProfileVisibilityRegisteredUsers,
		ProfileVisibilityPublic,
	}
}

func (e ProfileVisibility) IsValid() error {
	switch e {
	case ProfileVisibilityConnections, ProfileVisibilityRegisteredUsers, ProfileVisibilityPublic:
		return nil
	default:
		return errors.New("enum is not valid")
	}
}

func (e ProfileVisibility) String() string {
	return string(e)
}

func (e ProfileVisibility) Ordinal() int {
	switch e {
	case ProfileVisibilityConnections:
		return 0
	case ProfileVisibilityRegisteredUsers:
		return 1
	case ProfileVisibilityPublic:
		return 2

	default:
		panic(errors.New("enum is not valid"))
	}
}
