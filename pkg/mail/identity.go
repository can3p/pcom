package mail

import "github.com/can3p/pcom/pkg/links"

// Identity is what the mail of a service needs to know about the installation:
// the site its links point to, the address mail comes from and the address
// of the admin the notifications go to.
type Identity struct {
	Site         links.Site
	From         string
	AdminAddress string
}
