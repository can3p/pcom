package mail

import (
	"fmt"

	"github.com/can3p/pcom/pkg/model"
	"github.com/google/uuid"
)

const anonymousUser = "Anonymous"

// AdminPageFailureInput is what the panic alert to the admin shows: who hit
// the page and the report of the request, the panic and its stack.
type AdminPageFailureInput struct {
	From         string
	AdminAddress string
	UniqueID     string
	User         string
	Report       string
}

// Header addresses the mail to the admin; every failure has its own id.
func (in AdminPageFailureInput) Header() Header {
	return Header{UniqueID: in.UniqueID, From: FromPcom(in.From), To: To(in.AdminAddress)}
}

var adminPageFailureMail = declare("panic_notification", "admin_page_failure", adminPageFailureSamples)

func adminPageFailureSamples() []Sample[AdminPageFailureInput] {
	const report = "[Recovery] 2026/01/02 - 03:04:05 panic recovered:\r\n" +
		"GET /posts/1 HTTP/1.1\r\nHost: pcom.test\r\nCookie: <hidden>\r\n\r\n" +
		"something broke\r\n" +
		"/app/pkg/web/handler.go:42 (0x1234)\r\n\tHandle: panic(err)\r\n"

	return []Sample[AdminPageFailureInput]{
		{Name: "admin_page_failure", Input: AdminPageFailureInput{
			From: SampleFrom, AdminAddress: sampleAdmin,
			UniqueID: "0190a3b4-0000-7000-8000-000000000004",
			User:     "email (alice@example.test), id (0190a3b4-0000-7000-8000-000000000001)", Report: report,
		}},
		{Name: "admin_page_failure_anonymous", Input: AdminPageFailureInput{
			From: SampleFrom, AdminAddress: sampleAdmin,
			UniqueID: "0190a3b4-0000-7000-8000-000000000004", User: anonymousUser, Report: report,
		}},
	}
}

// AdminPageFailure tells the admin that a page panicked. report is the
// request, the panic value and the stack as the caller formatted them; user
// is nil for a visitor who is not logged in.
func AdminPageFailure(from, adminAddress, report string, user *model.User) *Envelope {
	who := anonymousUser
	if user != nil {
		who = fmt.Sprintf("email (%s), id (%s)", user.Email, user.ID)
	}

	return adminPageFailureMail.MustRender(AdminPageFailureInput{
		From: from, AdminAddress: adminAddress, UniqueID: uuid.NewString(), User: who, Report: report,
	})
}
