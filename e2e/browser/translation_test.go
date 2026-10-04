//go:build browser

// Translating posts and RSS items into English: the reader's button, the
// label every translation carries, switching back to the original, the
// languages a reader always translates and who gets no control at all. The
// translation service is WireMock, with the stubs in
// testdata/wiremock/mappings.
package browser_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/can3p/pcom/e2e"
	"github.com/can3p/pcom/e2e/browser"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/testutil/factory"
	"github.com/can3p/pcom/pkg/testutil/wiremock"
	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
	wm "github.com/wiremock/go-wiremock"
)

const translatedLabel = "Automatically translated from German by Azure Translator"

// translationApp starts the app against WireMock; a call no stub matches
// fails the test.
// markers are German text only this test sends, so another test's unmatched
// call doesn't fail it.
func translationApp(t *testing.T, markers ...string) *e2e.App {
	t.Helper()

	wiremock.Shared(t).Watch(t, "azure-translator", markers...)

	return e2e.Start(t, e2e.WithRealAssets(), e2e.WithWireMock())
}

// countRequests counts the page's requests whose URL contains part.
func countRequests(page playwright.Page, part string) *atomic.Int32 {
	var n atomic.Int32

	page.OnRequest(func(r playwright.Request) {
		if strings.Contains(r.URL(), part) {
			n.Add(1)
		}
	})

	return &n
}

// germanPost is a published German post of author that the reader's
// connection to author lets them see.
func germanPost(t *testing.T, app *e2e.App, author *core.User, subject, body string, opts ...factory.PostOpt) *core.Post {
	t.Helper()

	opts = append([]factory.PostOpt{
		factory.Published(), factory.WithLanguage("de"), factory.WithSubject(subject), factory.WithBody(body),
	}, opts...)

	post, err := factory.Post(context.Background(), app.DB, author.ID, opts...)
	require.NoError(t, err)

	return post
}

// connectedReader is a new user connected to author.
func connectedReader(t *testing.T, app *e2e.App, author *core.User) *core.User {
	t.Helper()

	reader := browser.NewUser(t, app)
	_, _, err := factory.Connect(context.Background(), app.DB, author.ID, reader.ID)
	require.NoError(t, err)

	return reader
}

func buttonNamed(page playwright.Page, name string) playwright.Locator {
	return page.GetByRole("button", playwright.PageGetByRoleOptions{Name: name, Exact: new(true)})
}

// alwaysTranslateGerman picks German in the settings card and saves it.
func alwaysTranslateGerman(t *testing.T, page playwright.Page) {
	t.Helper()

	_, err := page.Goto("/controls/settings")
	require.NoError(t, err)

	_, err = page.GetByLabel("Always translate these languages").SelectOption(playwright.SelectOptionValues{Values: &[]string{"de"}})
	require.NoError(t, err)
	require.NoError(t, buttonNamed(page, "Save").Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText("Translation settings have been saved")).ToBeVisible())
}

func feedPostOf(page playwright.Page, post *core.Post) playwright.Locator {
	return page.Locator(".us-feed-post").Filter(playwright.LocatorFilterOptions{HasText: post.Subject.String})
}

// A reader translates a German post: the English keeps the list and the link,
// the label names the language and the provider, and "Show original" brings
// the German back without leaving the page.
func TestTranslation_TranslateAndShowOriginal(t *testing.T) {
	t.Parallel()

	app := translationApp(t, "Tobias")
	author := browser.NewUser(t, app)
	reader := connectedReader(t, app, author)
	post := germanPost(t, app, author, "Samstag mit Tobias",
		"Einkaufsliste für Tobias:\n\n- Äpfel kaufen\n- [Brot](https://example.com/brot) holen\n", factory.AllowTranslation())

	page := browser.Page(t, app, browser.As(reader))
	_, err := page.Goto("/posts/" + post.ID)
	require.NoError(t, err)

	original := page.Locator(`[lang="de"]`)
	require.NoError(t, browser.Expect.Locator(original).ToContainText("Äpfel kaufen"))
	require.NoError(t, buttonNamed(page, "Translate").Click())

	text := page.Locator(".translation-text")
	require.NoError(t, browser.Expect.Locator(page.GetByText(translatedLabel)).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(text).ToContainText("Saturday with Tobias"))
	require.NoError(t, browser.Expect.Locator(text.Locator("ul > li")).ToHaveCount(2))
	require.NoError(t, browser.Expect.Locator(text.Locator("li").First()).ToHaveText("Buy apples"))
	require.NoError(t, browser.Expect.Locator(text.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Get bread"})).
		ToHaveAttribute("href", "https://example.com/brot"))
	require.NoError(t, browser.Expect.Locator(original).ToHaveCount(0))

	require.NoError(t, buttonNamed(page, "Show original").Click())
	require.NoError(t, browser.Expect.Locator(original).ToContainText("Äpfel kaufen"))
	require.NoError(t, browser.Expect.Locator(text).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(page.GetByText(translatedLabel)).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(buttonNamed(page, "Translate")).ToBeVisible())
}

// With German in "always translate" the feed shows the translation first, with
// the label, and the original is one click away.
func TestTranslation_AlwaysTranslateShowsTranslationFirst(t *testing.T) {
	t.Parallel()

	app := translationApp(t, "Bremen")
	author := browser.NewUser(t, app)
	reader := connectedReader(t, app, author)
	post := germanPost(t, app, author, "Lagebericht aus Bremen",
		"Heute hat es in Bremen den ganzen Tag geregnet.", factory.AllowTranslation())

	page := browser.Page(t, app, browser.As(reader))
	alwaysTranslateGerman(t, page)

	_, err := page.Goto("/feed")
	require.NoError(t, err)

	card := feedPostOf(page, post)
	require.NoError(t, browser.Expect.Locator(card.GetByText(translatedLabel)).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(card.Locator(".translation-text")).ToContainText("It rained all day in Bremen today."))
	require.NoError(t, browser.Expect.Locator(card).Not().ToContainText("den ganzen Tag geregnet"))

	require.NoError(t, card.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Show original"}).Click())
	require.NoError(t, browser.Expect.Locator(card).ToContainText("den ganzen Tag geregnet"))
	require.NoError(t, browser.Expect.Locator(card.GetByText(translatedLabel)).ToHaveCount(0))
}

// An anonymous visitor reads the post as it is, without a control.
func TestTranslation_AnonymousSeesNoControl(t *testing.T) {
	t.Parallel()

	app := translationApp(t, "Kassel")
	author := browser.NewUser(t, app)
	post := germanPost(t, app, author, "Ein öffentlicher Gruß aus Kassel", "Liebe Grüße aus Kassel an alle.",
		factory.AllowTranslation(), factory.Visibility(core.PostVisibilityPublic))

	page := browser.Page(t, app)
	_, err := page.Goto("/posts/" + post.ID)
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(page.Locator(`[lang="de"]`)).ToContainText("Liebe Grüße aus Kassel"))
	require.NoError(t, browser.Expect.Locator(buttonNamed(page, "Translate")).ToHaveCount(0))
}

// A post whose author didn't allow translation has no control, and is never
// translated automatically, while another post of the same feed is.
func TestTranslation_NotAllowedByAuthor(t *testing.T) {
	t.Parallel()

	app := translationApp(t, "Dresden", "Leipzig")
	author := browser.NewUser(t, app)
	reader := connectedReader(t, app, author)
	closed := germanPost(t, app, author, "Geheimes Rezept aus Dresden", "Das Rezept bleibt in Dresden.")
	open := germanPost(t, app, author, "Wetterbericht aus Leipzig", "In Leipzig scheint die Sonne.", factory.AllowTranslation())

	page := browser.Page(t, app, browser.As(reader))
	alwaysTranslateGerman(t, page)

	closedRequests := countRequests(page, "/controls/translate/post/"+closed.ID)

	_, err := page.Goto("/feed")
	require.NoError(t, err)

	require.NoError(t, browser.Expect.Locator(feedPostOf(page, open).GetByText(translatedLabel)).ToBeVisible())

	card := feedPostOf(page, closed)
	require.NoError(t, browser.Expect.Locator(card).ToContainText("Das Rezept bleibt in Dresden."))
	require.NoError(t, browser.Expect.Locator(card.GetByText(translatedLabel)).ToHaveCount(0))
	require.NoError(t, browser.Expect.Locator(card.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Translate"})).ToHaveCount(0))
	require.Zero(t, closedRequests.Load())
}

// The author ticks the toggle, and a reader can then translate the post.
func TestTranslation_AuthorAllowsTranslation(t *testing.T) {
	t.Parallel()

	app := translationApp(t, "Mainz")
	author := browser.NewUser(t, app)
	reader := connectedReader(t, app, author)
	post := germanPost(t, app, author, "Neuigkeiten aus Mainz",
		"Der Karneval in Mainz beginnt jedes Jahr im November, und die ganze Stadt feiert gemeinsam auf den Straßen und in den vielen kleinen Kneipen der Altstadt.")

	readerPage := browser.Page(t, app, browser.As(reader))
	_, err := readerPage.Goto("/posts/" + post.ID)
	require.NoError(t, err)
	require.NoError(t, browser.Expect.Locator(readerPage.Locator(`[lang="de"]`)).ToContainText("Karneval in Mainz"))
	require.NoError(t, browser.Expect.Locator(buttonNamed(readerPage, "Translate")).ToHaveCount(0))

	authorPage := browser.Page(t, app, browser.As(author))
	_, err = authorPage.Goto("/posts/" + post.ID + "/edit")
	require.NoError(t, err)
	require.NoError(t, authorPage.GetByLabel("Allow readers to translate this post").Check())
	require.NoError(t, buttonNamed(authorPage, "Save Post").Click())
	require.NoError(t, browser.Expect.Locator(authorPage.Locator(".us-post-header")).ToContainText("Neuigkeiten aus Mainz"))

	_, err = readerPage.Goto("/posts/" + post.ID)
	require.NoError(t, err)
	require.NoError(t, buttonNamed(readerPage, "Translate").Click())
	require.NoError(t, browser.Expect.Locator(readerPage.GetByText(translatedLabel)).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(readerPage.Locator(".translation-text")).ToContainText("The carnival in Mainz begins every November"))
}

// A second reader of the same post gets the stored translation: WireMock saw
// one call.
func TestTranslation_SecondReaderMakesNoNewCall(t *testing.T) {
	t.Parallel()

	app := translationApp(t, "Ulm")
	author := browser.NewUser(t, app)
	post := germanPost(t, app, author, "Gartenarbeit in Ulm",
		"Im Garten in Ulm wachsen dieses Jahr besonders viele Tomaten.", factory.AllowTranslation())

	// the container outlives a test, so with -count the call is counted from here
	calls := wm.NewRequest("POST", wm.URLPathEqualTo("/azure-translator/translate")).WithBodyPattern(wm.Contains("Gartenarbeit in Ulm"))
	before, err := wiremock.Shared(t).Client().GetCountRequests(calls)
	require.NoError(t, err)

	for range 2 {
		page := browser.Page(t, app, browser.As(connectedReader(t, app, author)))
		_, err := page.Goto("/posts/" + post.ID)
		require.NoError(t, err)
		require.NoError(t, buttonNamed(page, "Translate").Click())
		require.NoError(t, browser.Expect.Locator(page.Locator(".translation-text")).ToContainText("tomatoes grow in the garden in Ulm"))
	}

	after, err := wiremock.Shared(t).Client().GetCountRequests(calls)
	require.NoError(t, err)
	require.EqualValues(t, 1, after-before)
}

// An RSS item in the feed is translated the same way.
func TestTranslation_RSSItem(t *testing.T) {
	t.Parallel()

	app := translationApp(t, "Hamburg")
	ctx := context.Background()
	reader := browser.NewUser(t, app)

	feed, err := factory.RSSFeed(ctx, app.DB)
	require.NoError(t, err)
	_, err = factory.Subscription(ctx, app.DB, reader.ID, feed.ID)
	require.NoError(t, err)
	item, err := factory.RSSItem(ctx, app.DB, feed.ID, factory.WithItemTitle("Hafenbericht aus Hamburg"),
		factory.WithItemLanguage("de"), factory.WithItemDescription("Der Hafen in Hamburg ist heute sehr belebt.\n\n- Große Schiffe\n- [Fähren](https://example.com/faehren) im Takt\n"))
	require.NoError(t, err)
	_, err = factory.UserFeedItem(ctx, app.DB, reader.ID, item.ID)
	require.NoError(t, err)

	page := browser.Page(t, app, browser.As(reader))
	_, err = page.Goto("/feed")
	require.NoError(t, err)

	card := page.Locator(".us-feed-rss-item")
	require.NoError(t, card.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Translate"}).Click())
	require.NoError(t, browser.Expect.Locator(card.GetByText(translatedLabel)).ToBeVisible())
	require.NoError(t, browser.Expect.Locator(card.Locator(".translation-text")).ToContainText("The harbour in Hamburg is very busy today."))
	require.NoError(t, browser.Expect.Locator(card.Locator(".translation-text ul > li")).ToHaveCount(2))
	require.NoError(t, browser.Expect.Locator(card.Locator(".translation-text").GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "Ferries"})).
		ToHaveAttribute("href", "https://example.com/faehren"))
}

// A reader without always-translate languages gets the Translate button with
// the page, and the page asks for no translation.
func TestTranslation_NoRequestOnLoadWithoutAlwaysTranslate(t *testing.T) {
	t.Parallel()

	app := translationApp(t, "Hannover")
	author := browser.NewUser(t, app)
	reader := connectedReader(t, app, author)
	post := germanPost(t, app, author, "Zugverbindung nach Hannover", "Der Zug nach Hannover fährt jede Stunde.", factory.AllowTranslation())

	page := browser.Page(t, app, browser.As(reader))

	translateRequests := countRequests(page, "/controls/translate")

	_, err := page.Goto("/feed")
	require.NoError(t, err)

	card := feedPostOf(page, post)
	require.NoError(t, browser.Expect.Locator(card.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Translate"})).ToBeVisible())
	require.NoError(t, page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateNetworkidle}))

	require.Zero(t, translateRequests.Load())
}

// A translation made once is in the page itself when the reader always
// translates its language: no request follows the load.
func TestTranslation_CachedAutoTranslationIsInTheInitialPage(t *testing.T) {
	t.Parallel()

	app := translationApp(t, "Kiel")
	author := browser.NewUser(t, app)
	reader := connectedReader(t, app, author)
	post := germanPost(t, app, author, "Bahnhof in Kiel",
		"Am Bahnhof in Kiel gibt es einen kleinen Bäcker.", factory.AllowTranslation())

	page := browser.Page(t, app, browser.As(reader))
	_, err := page.Goto("/posts/" + post.ID)
	require.NoError(t, err)
	require.NoError(t, buttonNamed(page, "Translate").Click())
	require.NoError(t, browser.Expect.Locator(page.GetByText(translatedLabel)).ToBeVisible())

	alwaysTranslateGerman(t, page)

	requests := countRequests(page, "/controls/translate")

	_, err = page.Goto("/feed")
	require.NoError(t, err)

	card := feedPostOf(page, post)
	require.NoError(t, browser.Expect.Locator(card.Locator(".translation-text")).ToContainText("There is a small baker at the station in Kiel."))
	require.NoError(t, page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{State: playwright.LoadStateNetworkidle}))
	require.Zero(t, requests.Load())
}
