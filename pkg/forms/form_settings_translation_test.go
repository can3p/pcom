package forms_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/can3p/pcom/pkg/forms"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/can3p/pcom/pkg/service/translations"
	"github.com/can3p/pcom/pkg/testutil"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/ginctx"
	"github.com/can3p/pcom/pkg/testutil/testdb"
	"github.com/stretchr/testify/require"
)

func TestSettingsTranslationForm(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	store := repo.New(db)
	svc := translations.New(store, reading.New(store), nil, translations.Limits{})
	user := testutil.Must(factory.User(ctx, db))(t)
	c, _ := ginctx.New(t, http.MethodPost, "/settings/translation", nil)

	bad := forms.SettingsTranslationNew(svc, user, nil)
	bad.Input.Languages = []string{"de", "xx"}
	require.Error(t, bad.Validate(c))
	require.True(t, bad.Errors.HasError("languages"))

	form := forms.SettingsTranslationNew(svc, user, nil)
	form.Input.Languages = []string{"de", "fr"}
	require.NoError(t, form.Validate(c))

	selected := map[string]bool{}

	for _, o := range form.TemplateData()["Options"].([]forms.TranslationLanguage) {
		selected[o.Code] = o.Selected
	}

	require.True(t, selected["de"])
	require.True(t, selected["fr"])
	require.False(t, selected["ru"])

	require.Equal(t, "German", forms.LanguageName("de"))
	require.Equal(t, "xx", forms.LanguageName("xx"))
}

// every language offered is one the service accepts.
func TestSettingsTranslationForm_OffersOnlyAcceptedLanguages(t *testing.T) {
	t.Parallel()

	db := testdb.New(t).DB
	ctx := context.Background()
	store := repo.New(db)
	svc := translations.New(store, reading.New(store), nil, translations.Limits{})
	user := testutil.Must(factory.User(ctx, db))(t)

	form := forms.SettingsTranslationNew(svc, user, nil)

	var codes []string
	for _, o := range form.TemplateData()["Options"].([]forms.TranslationLanguage) {
		codes = append(codes, o.Code)
	}

	require.NoError(t, svc.SetLanguages(ctx, user, codes))
}
