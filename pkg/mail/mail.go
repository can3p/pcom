// Package mail formats the mail pcom sends. It builds messages and never
// sends them: a service queues the Envelope a constructor returns with
// tx.SendMail, in the transaction of the change it reports.
//
// # Declaring a mail
//
// Every mail lives in its own file, pkg/mail/<name>.go, where <name> is its
// template name (snake_case, unique across the package). The file holds:
//
//   - the input, a struct <Name>Input with everything the templates show and
//     a Header method returning the unique id, the from address and the
//     recipients; values the templates need derived (minutes from a duration,
//     an absolute link) are methods on the input, so the templates stay
//     logic-free;
//   - the declaration, a package-level
//     var <name>Mail = declare("<email_type>", "<name>", <name>Samples),
//     where <email_type> is the outgoing queue's email_type. declare
//     registers the mail itself, so adding a mail touches no shared list;
//   - the samples, func <name>Samples() []Sample[<Name>Input], returning named
//     inputs built from in-memory model structs with fixed values (SampleFrom,
//     SampleSite, literal ids, emails and usernames), never a database. A
//     sample is named after the golden it reproduces,
//     testdata/<sample>.golden, and sample names are unique across the
//     package;
//   - the public constructor, which keeps its signature: it returns nil
//     before rendering when there is nobody to notify, builds the input and
//     returns <name>Mail.Render(in), or <name>Mail.MustRender(in) when its
//     signature has no error.
//
// # Templates
//
// The templates are embedded from pkg/mail/templates/: <name>.subject.txt and
// <name>.txt are text/template, <name>.html is html/template; the input is
// the dot. They are parsed once, when declare runs at package init, and a
// missing or broken template panics there rather than at send time. Render
// trims the trailing whitespace of each part, so a template file may end with
// a newline, and the leading whitespace of the subject, which is one line.
// The leading whitespace of the text and HTML parts is kept: a mail whose
// golden starts with a blank line has a template that starts with one.
//
// html/template escapes every value, so user content (usernames, subjects,
// comment text, links) goes into the input as a plain string. A field of type
// template.HTML is the one way trusted HTML enters a template: a markdown
// body rendered with markdown.ToEnrichedTemplate is such a field, and nothing
// else is wrapped in template.HTML.
//
// # Registry
//
// All lists every declared mail with its samples rendered, sorted by template
// name. It is the only list of mails: the golden test (TestMails_Goldens)
// renders every sample into its golden file and the preview page shows them.
package mail

import (
	"bytes"
	"embed"
	htmltemplate "html/template"
	"io"
	"maps"
	"net/mail"
	"slices"
	"strings"
	texttemplate "text/template"
	"unicode"

	"github.com/can3p/gogo/sender"
	"github.com/can3p/pcom/pkg/links"
	"github.com/pkg/errors"
)

// Envelope is a mail with the id and type the outgoing queue files it under.
// The functions of this package build messages; the services send them.
type Envelope struct {
	UniqueID string
	Type     string
	Mail     *sender.Mail
}

// Header is the addressing of one mail: the unique id the queue drops repeats
// by, and who it is from and to.
type Header struct {
	UniqueID string
	From     mail.Address
	To       []mail.Address
}

// Input is what a declared mail renders: the template data, which also knows
// its own addressing.
type Input interface {
	Header() Header
}

// Sample is a named input a mail is previewed and golden-tested with.
type Sample[In Input] struct {
	Name  string
	Input In
}

// SampleFrom is the from address samples use.
const SampleFrom = "noreply@pcom.test"

// SampleSite is the site samples format their links for.
var SampleSite = links.Site{Root: "https://pcom.test"}

// FromPcom is the from address of every pcom mail, at address from.
func FromPcom(from string) mail.Address {
	return mail.Address{Address: from, Name: "Your pcom"}
}

// To is the recipient list of a mail to one address.
func To(address string) []mail.Address {
	return []mail.Address{{Address: address}}
}

//go:embed templates
var templatesFS embed.FS

// Definition is one declared mail: its email type, its parsed templates and
// its samples.
type Definition[In Input] struct {
	emailType string
	name      string
	subject   *texttemplate.Template
	text      *texttemplate.Template
	html      *htmltemplate.Template
	samples   func() []Sample[In]
}

// Type is the queue's email_type of the mail.
func (d *Definition[In]) Type() string { return d.emailType }

// Name is the template name of the mail.
func (d *Definition[In]) Name() string { return d.name }

// Render executes the templates on in and addresses the result.
func (d *Definition[In]) Render(in In) (*Envelope, error) {
	subject, err := execute(d.subject, in)
	if err != nil {
		return nil, errors.Wrapf(err, "mail %s: subject", d.name)
	}

	text, err := execute(d.text, in)
	if err != nil {
		return nil, errors.Wrapf(err, "mail %s: text", d.name)
	}

	html, err := execute(d.html, in)
	if err != nil {
		return nil, errors.Wrapf(err, "mail %s: html", d.name)
	}

	subject = strings.TrimSpace(subject)
	h := in.Header()

	return &Envelope{
		UniqueID: h.UniqueID,
		Type:     d.emailType,
		Mail: &sender.Mail{
			From:    h.From,
			To:      h.To,
			Subject: subject,
			Text:    text,
			Html:    html,
		},
	}, nil
}

// MustRender is Render for constructors that cannot return an error. A
// template that fails on a well-formed input is a programming error, which
// the golden test catches, so it panics.
func (d *Definition[In]) MustRender(in In) *Envelope {
	e, err := d.Render(in)
	if err != nil {
		panic(err)
	}

	return e
}

func execute(t interface {
	Execute(w io.Writer, data any) error
}, data any) (string, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return strings.TrimRightFunc(buf.String(), unicode.IsSpace), nil
}

// RenderedSample is one sample of a registered mail, rendered.
type RenderedSample struct {
	Name     string
	Envelope *Envelope
	Err      error
}

// Registered is one declared mail as All lists it.
type Registered struct {
	Type    string
	Name    string
	Samples []RenderedSample
}

type registration interface {
	register() Registered
}

func (d *Definition[In]) register() Registered {
	r := Registered{Type: d.emailType, Name: d.name}

	for _, s := range d.samples() {
		e, err := d.Render(s.Input)
		r.Samples = append(r.Samples, RenderedSample{Name: s.Name, Envelope: e, Err: err})
	}

	return r
}

// registry holds every declared mail by name. declare fills it during
// package init, before anything reads it.
var registry = map[string]registration{}

// declare parses the templates of the mail name and registers it. It panics
// when a template is missing or does not parse, or when name is declared
// twice, so a broken mail stops the binary at init.
func declare[In Input](emailType, name string, samples func() []Sample[In]) *Definition[In] {
	d := &Definition[In]{
		emailType: emailType,
		name:      name,
		subject:   texttemplate.Must(texttemplate.New(name+".subject.txt").ParseFS(templatesFS, "templates/"+name+".subject.txt")),
		text:      texttemplate.Must(texttemplate.New(name+".txt").ParseFS(templatesFS, "templates/"+name+".txt")),
		html:      htmltemplate.Must(htmltemplate.New(name+".html").ParseFS(templatesFS, "templates/"+name+".html")),
		samples:   samples,
	}

	if _, ok := registry[name]; ok {
		panic("mail " + name + " is declared twice")
	}

	registry[name] = d

	return d
}

// All lists every declared mail with its samples rendered, sorted by name.
func All() []Registered {
	names := slices.Sorted(maps.Keys(registry))

	out := make([]Registered, 0, len(names))
	for _, name := range names {
		out = append(out, registry[name].register())
	}

	return out
}
