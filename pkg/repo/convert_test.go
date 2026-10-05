package repo

import (
	"reflect"
	"testing"
	"time"

	"github.com/can3p/pcom/pkg/model"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/stretchr/testify/require"
)

// convertPairs is every core type with its model type.
var convertPairs = []struct{ core, model any }{
	{&core.LoginAttempt{}, &model.LoginAttempt{}},
	{&core.MediaUpload{}, &model.MediaUpload{}},
	{&core.NormalizedURL{}, &model.NormalizedURL{}},
	{&core.OutgoingEmail{}, &model.OutgoingEmail{}},
	{&core.Post{}, &model.Post{}},
	{&core.PostComment{}, &model.PostComment{}},
	{&core.PostPrompt{}, &model.PostPrompt{}},
	{&core.PostShare{}, &model.PostShare{}},
	{&core.PostStat{}, &model.PostStat{}},
	{&core.RSSFeed{}, &model.RSSFeed{}},
	{&core.RSSItem{}, &model.RSSItem{}},
	{&core.SystemSetting{}, &model.SystemSetting{}},
	{&core.User{}, &model.User{}},
	{&core.UserAPIKey{}, &model.UserAPIKey{}},
	{&core.UserConnection{}, &model.UserConnection{}},
	{&core.UserConnectionMediationRequest{}, &model.UserConnectionMediationRequest{}},
	{&core.UserConnectionMediator{}, &model.UserConnectionMediator{}},
	{&core.UserFeedItem{}, &model.UserFeedItem{}},
	{&core.UserFeedSubscription{}, &model.UserFeedSubscription{}},
	{&core.UserFeedToken{}, &model.UserFeedToken{}},
	{&core.UserInvitation{}, &model.UserInvitation{}},
	{&core.UserProfile{}, &model.UserProfile{}},
	{&core.UserSignupRequest{}, &model.UserSignupRequest{}},
	{&core.UserStyle{}, &model.UserStyle{}},
	{&core.WhitelistedConnection{}, &model.WhitelistedConnection{}},
}

// TestConvert_RoundTripLosesNothing fills every column field and every
// relation the model carries with non-zero values, converts to the model and
// back, and expects the same struct.
func TestConvert_RoundTripLosesNothing(t *testing.T) {
	t.Parallel()

	for _, p := range convertPairs {
		ct, mt := reflect.TypeOf(p.core).Elem(), reflect.TypeOf(p.model).Elem()
		t.Run(ct.Name(), func(t *testing.T) {
			t.Parallel()

			src := reflect.New(ct)
			fill(src.Elem(), mt)

			m := reflect.New(mt)
			convert(m, src, map[uintptr]reflect.Value{})
			for i := range mt.NumField() {
				f := mt.Field(i)
				if isRelation(f) {
					require.False(t, m.Elem().Field(i).IsNil(), "relation %s was not converted", f.Name)
				} else if f.IsExported() && !f.Anonymous {
					require.False(t, m.Elem().Field(i).IsZero(), "field %s was not converted", f.Name)
				}
			}

			back := reflect.New(ct)
			convert(back, m, map[uintptr]reflect.Value{})
			require.Equal(t, src.Interface(), back.Interface())
		})
	}
}

// TestConvert_BackReferences converts the cycle sqlboiler builds when it
// loads a relation in both directions, and keeps it a cycle.
func TestConvert_BackReferences(t *testing.T) {
	t.Parallel()

	post := &core.Post{ID: "p"}
	stat := &core.PostStat{ID: "s", PostID: "p"}
	post.R = post.R.NewStruct()
	post.R.PostStat = stat
	stat.R = stat.R.NewStruct()
	stat.R.Post = post

	got := toModel[model.Post](post)
	require.Same(t, got, got.PostStat.Post)

	back := toCore[core.Post](got)
	require.Same(t, back, back.R.PostStat.R.Post)
}

func TestConvert_NilsAndSlices(t *testing.T) {
	t.Parallel()

	require.Nil(t, toModel[model.Post]((*core.Post)(nil)))
	require.Nil(t, toModels[model.Post](core.PostSlice(nil)))

	got := toModels[model.Post](core.PostSlice{{ID: "a"}, {ID: "b"}})
	require.Len(t, got, 2)
	require.Equal(t, "b", got[1].ID)

	// No relation on the model leaves the core struct's R nil.
	require.Nil(t, toCore[core.Post](&model.Post{ID: "a"}).R)

	m := &model.Post{ID: "a"}
	copyInto(m, &core.Post{ID: "a", Body: "from the database"})
	require.Equal(t, "from the database", m.Body)
}

// fill sets every column field of the core struct v to a non-zero value and,
// for each relation modelType carries, R's field to a filled related struct.
func fill(v reflect.Value, modelType reflect.Type) {
	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		switch {
		case f.Name == "L" || !f.IsExported():
		case f.Name == "R":
			if modelType == nil {
				continue
			}
			r := reflect.New(f.Type.Elem())
			any := false
			for mf := range modelType.Fields() {
				if !isRelation(mf) {
					continue
				}
				rf := r.Elem().FieldByName(coreRelName(t, mf.Name))
				if rf.Type().Kind() == reflect.Slice {
					el := reflect.New(rf.Type().Elem().Elem())
					fill(el.Elem(), nil)
					rf.Set(reflect.Append(rf, el))
				} else {
					el := reflect.New(rf.Type().Elem())
					fill(el.Elem(), nil)
					rf.Set(el)
				}
				any = true
			}
			if any {
				v.Field(i).Set(r)
			}
		default:
			fillValue(v.Field(i), f.Name)
		}
	}
}

func fillValue(v reflect.Value, name string) {
	switch {
	case v.Type() == reflect.TypeFor[time.Time]():
		v.Set(reflect.ValueOf(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)))
	case isNull(v.Type()):
		fillValue(v.Field(0), name)
		v.FieldByName("Valid").SetBool(true)
	case v.Kind() == reflect.String:
		v.SetString("value of " + name)
	case v.Kind() == reflect.Int || v.Kind() == reflect.Int64:
		v.SetInt(7)
	case v.Kind() == reflect.Float64:
		v.SetFloat(1.5)
	case v.Kind() == reflect.Bool:
		v.SetBool(true)
	case v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8:
		v.SetBytes([]byte(`{"a":1}`))
	default:
		panic("fill: no value for " + v.Type().String())
	}
}
