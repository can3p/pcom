package translations_test

import (
	"context"
	"testing"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/service/translations"
	"github.com/can3p/pcom/pkg/translate"
	"github.com/stretchr/testify/require"
)

func TestSkeleton(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	off := translations.New(nil, nil, translations.Limits{})
	require.False(t, off.Enabled())

	s := translations.New(nil, &translate.Translator{}, translations.Limits{UserDailyChars: 1, SiteMonthlyChars: 2})
	require.True(t, s.Enabled())

	_, err := s.Translate(ctx, nil, core.TranslationSourceKindPost, "p")
	require.ErrorIs(t, err, translations.ErrNotImplemented)
	require.ErrorIs(t, s.RetranslateStale(ctx, nil, "p"), translations.ErrNotImplemented)
	require.ErrorIs(t, s.Forget(ctx, nil, "p"), translations.ErrNotImplemented)
}
