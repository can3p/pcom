package mail

import (
	"testing"

	"github.com/can3p/pcom/pkg/links"
	"github.com/can3p/pcom/pkg/model"
	"github.com/stretchr/testify/require"
)

func TestConfirmWaitingList_MatchesSample(t *testing.T) {
	t.Parallel()

	waitingList := &model.UserSignupRequest{ID: "request-1", Email: "newuser@example.test"}
	want := confirmWaitingListMail.MustRender(confirmWaitingListSamples()[0].Input)

	require.Equal(t, want, ConfirmWaitingList(links.Site{}, SampleFrom, waitingList))
}

func TestInvitation_MatchesSample(t *testing.T) {
	t.Parallel()

	invite := &model.UserInvitation{ID: "invite-1"}
	want := inviteMail.MustRender(inviteSamples()[0].Input)

	require.Equal(t, want, Invitation(links.Site{}, SampleFrom, invite, "newuser@example.test"))
}

func TestValidateFormat_InvalidFormat(t *testing.T) {
	t.Parallel()

	err := ValidateFormat("not-a-valid-email")
	require.Error(t, err)
	require.Contains(t, err.Error(), "Invalid email format")
}

func TestValidateFormat_ValidFormat(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidateFormat("brand-new-user@example.test"))
}
