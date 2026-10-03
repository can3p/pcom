package forms

import (
	"context"
	"slices"

	"github.com/can3p/gogo/forms"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service/translations"
	"github.com/can3p/pcom/pkg/util/formhelpers"
	"github.com/gin-gonic/gin"
)

// TranslationLanguage is a language offered for translation.
type TranslationLanguage struct {
	Code     string // ISO 639-1
	Name     string
	Selected bool
}

// translationLanguages are the common languages offered in the settings, in
// the order shown, and the names the translation label uses. English is not
// among them: it is what is translated into.
var translationLanguages = []TranslationLanguage{
	{Code: "ar", Name: "Arabic"},
	{Code: "bg", Name: "Bulgarian"},
	{Code: "zh", Name: "Chinese"},
	{Code: "cs", Name: "Czech"},
	{Code: "da", Name: "Danish"},
	{Code: "nl", Name: "Dutch"},
	{Code: "fi", Name: "Finnish"},
	{Code: "fr", Name: "French"},
	{Code: "de", Name: "German"},
	{Code: "el", Name: "Greek"},
	{Code: "he", Name: "Hebrew"},
	{Code: "hi", Name: "Hindi"},
	{Code: "hu", Name: "Hungarian"},
	{Code: "id", Name: "Indonesian"},
	{Code: "it", Name: "Italian"},
	{Code: "ja", Name: "Japanese"},
	{Code: "ko", Name: "Korean"},
	{Code: "pl", Name: "Polish"},
	{Code: "pt", Name: "Portuguese"},
	{Code: "ro", Name: "Romanian"},
	{Code: "ru", Name: "Russian"},
	{Code: "es", Name: "Spanish"},
	{Code: "sv", Name: "Swedish"},
	{Code: "th", Name: "Thai"},
	{Code: "tr", Name: "Turkish"},
	{Code: "uk", Name: "Ukrainian"},
	{Code: "vi", Name: "Vietnamese"},
}

// LanguageName is the English name of an ISO 639-1 code, or the code itself
// for a language outside the common table.
func LanguageName(code string) string {
	for _, l := range translationLanguages {
		if l.Code == code {
			return l.Name
		}
	}

	return code
}

func languageOptions(selected []string) []TranslationLanguage {
	out := slices.Clone(translationLanguages)
	for i := range out {
		out[i].Selected = slices.Contains(selected, out[i].Code)
	}

	return out
}

type SettingsTranslationInput struct {
	Languages []string `form:"languages"`
}

type SettingsTranslation struct {
	*forms.FormBase[SettingsTranslationInput]
	Translations *translations.Service
	User         *core.User
}

// SettingsTranslationNew builds the form of the languages a reader always
// wants translated; selected are the ones stored now.
func SettingsTranslationNew(svc *translations.Service, u *core.User, selected []string) *SettingsTranslation {
	return &SettingsTranslation{
		FormBase: &forms.FormBase[SettingsTranslationInput]{
			Name:                "settings_translation",
			FormTemplate:        "form--settings-translation",
			KeepValuesAfterSave: true,
			Input:               &SettingsTranslationInput{Languages: selected},
			ExtraTemplateData:   map[string]any{"Options": languageOptions(selected)},
		},
		Translations: svc,
		User:         u,
	}
}

func (f *SettingsTranslation) Validate(c *gin.Context) error {
	for _, code := range f.Input.Languages {
		if LanguageName(code) == code {
			f.AddError("languages", "Pick the languages from the list.")
			break
		}
	}

	f.ExtraTemplateData["Options"] = languageOptions(f.Input.Languages)

	return f.Errors.PassedValidation()
}

func (f *SettingsTranslation) Save(c context.Context) (forms.FormSaveAction, error) {
	if err := f.Translations.SetLanguages(c, f.User, f.Input.Languages); err != nil {
		return nil, err
	}

	return formhelpers.SuccessBadge("Translation settings have been saved!"), nil
}
