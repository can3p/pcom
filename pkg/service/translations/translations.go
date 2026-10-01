// Package translations translates posts and RSS items into English for
// readers: it caches translations, keeps the per-reader daily and site-wide
// monthly character budgets, and re-translates edited posts in the
// background through a queue its worker sends, as mail is sent.
package translations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/abadojack/whatlanggo"
	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops/rss"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/can3p/pcom/pkg/service"
	"github.com/can3p/pcom/pkg/service/reading"
	"github.com/can3p/pcom/pkg/translate"
	"github.com/volatiletech/null/v8"
	"golang.org/x/sync/singleflight"
)

// TargetLang is the only language translated into, for now.
const TargetLang = "en"

var (
	// ErrNotTranslatable: the source gets no translation control. Its author
	// doesn't allow translation, it is already in English or its language is
	// unknown, or translation is off. It wraps service.ErrForbidden.
	ErrNotTranslatable = fmt.Errorf("%w: not translatable", service.ErrForbidden)
	// ErrLimit: the reader's daily or the site's monthly budget would be
	// exceeded; shown as "Translation is not available right now".
	ErrLimit = errors.New("translations: character limit reached")
	// ErrUpdating: the post was edited and its translation is queued to be
	// re-made; shown as "Translation is being updated".
	ErrUpdating = errors.New("translations: translation is being updated")
)

// DefaultPollInterval is how often the worker looks for due jobs.
const DefaultPollInterval = 30 * time.Second

// retryIntervals are the waits after each failed job attempt; a job that
// fails once more is marked failed, and the next reader re-makes it.
var retryIntervals = []time.Duration{time.Minute, 10 * time.Minute, time.Hour}

// Limits are the character budgets: a reader's per day and the site's per
// calendar month. Background re-translations count only against the site's.
type Limits struct {
	UserDailyChars   int
	SiteMonthlyChars int
}

// Service translates. Its translator is nil when translation is off.
type Service struct {
	store      *repo.Store
	reading    *reading.Service
	translator *translate.Translator
	limits     Limits
	flights    singleflight.Group
}

// New returns the service; translator nil turns translation off. Who may
// read a source is reading's rule.
func New(store *repo.Store, reading *reading.Service, translator *translate.Translator, limits Limits) *Service {
	return &Service{store: store, reading: reading, translator: translator, limits: limits}
}

// Enabled reports whether a backend is configured.
func (s *Service) Enabled() bool { return s.translator != nil }

// Result is a translation shown to a reader.
type Result struct {
	Subject string
	// Body is markdown for a post and sanitized HTML for an RSS item, as the source.
	Body string
	// SourceLang is the source's language, ISO 639-1; empty if unknown.
	SourceLang string
	// Provider is the backend's DisplayName, such as "Azure Translator".
	Provider string
}

// source is the text of a post or an RSS item to translate.
type source struct {
	kind    core.TranslationSourceKind
	id      string
	lang    string
	subject string
	body    string
}

func postSource(p *core.Post) *source {
	return &source{kind: core.TranslationSourceKindPost, id: p.ID, lang: p.Language.String, subject: p.Subject.String, body: p.Body}
}

func rssSource(i *core.RSSItem) *source {
	return &source{kind: core.TranslationSourceKindRSSItem, id: i.ID, lang: i.Language.String, subject: i.Title, body: i.SanitizedDescription}
}

// SourceHash is the hex SHA-256 of a source's language, subject and body; a
// cached translation with another hash is stale, so a re-detected language
// re-makes it too.
func SourceHash(lang, subject, body string) string {
	sum := sha256.Sum256([]byte(lang + "\x00" + subject + "\x00" + body))
	return hex.EncodeToString(sum[:])
}

func (src *source) hash() string { return SourceHash(src.lang, src.subject, src.body) }

// translatableLang reports whether text in lang gets a translation control:
// a known language other than English.
func translatableLang(lang null.String) bool {
	return lang.Valid && lang.String != "" && lang.String != TargetLang
}

// TranslatablePost reports whether the actor gets a translation control on
// a post they can see: translation is on, they are logged in, the post is
// published, its author allows translation and it is in a known language
// other than English.
func (s *Service) TranslatablePost(actor *core.User, p *core.Post) bool {
	return s.Enabled() && actor != nil && p.PublishedAt.Valid && p.AllowTranslation && translatableLang(p.Language)
}

// TranslatableRSSItem reports whether the actor gets a translation control
// on an RSS item of their feed.
func (s *Service) TranslatableRSSItem(actor *core.User, language null.String) bool {
	return s.Enabled() && actor != nil && translatableLang(language)
}

// Translate returns the English translation of a post or an RSS item the
// actor may read, from the cache or the backend. Anonymous is
// ErrNeedsLogin; a source the actor may not read, a draft included, is
// ErrNotFound; one without a control is ErrNotTranslatable. Over a budget it
// is ErrLimit, and an edited post whose re-translation is queued is
// ErrUpdating. A cache hit is free; a backend error stores and counts nothing.
func (s *Service) Translate(ctx context.Context, actor *core.User, kind core.TranslationSourceKind, id string) (*Result, error) {
	src, err := s.source(ctx, actor, kind, id)
	if err != nil {
		return nil, err
	}

	row, err := s.store.Translation(ctx, kind, id, TargetLang)
	if err != nil {
		return nil, err
	}

	if row != nil && row.SourceHash == src.hash() {
		return s.result(src.lang, row), nil
	}

	if row != nil && kind == core.TranslationSourceKindPost {
		job, err := s.store.PendingTranslationJob(ctx, id, TargetLang)
		if err != nil {
			return nil, err
		}

		if job != nil {
			return nil, ErrUpdating
		}
	}

	row, err = s.miss(ctx, src, actor.ID)
	if err != nil {
		return nil, err
	}

	return s.result(src.lang, row), nil
}

// source checks that the actor may translate kind/id and returns its text.
func (s *Service) source(ctx context.Context, actor *core.User, kind core.TranslationSourceKind, id string) (*source, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	switch kind {
	case core.TranslationSourceKindPost:
		post, err := s.reading.PostToTranslate(ctx, actor, id)
		if err != nil {
			return nil, err
		}

		if !s.TranslatablePost(actor, post) {
			return nil, ErrNotTranslatable
		}

		return postSource(post), nil
	case core.TranslationSourceKindRSSItem:
		item, err := s.reading.RSSItemToTranslate(ctx, actor, id)
		if err != nil {
			return nil, err
		}

		if !s.TranslatableRSSItem(actor, item.Language) {
			return nil, ErrNotTranslatable
		}

		return rssSource(item), nil
	default:
		return nil, service.ErrNotFound
	}
}

func (s *Service) result(lang string, row *core.Translation) *Result {
	provider := row.Provider
	if s.translator != nil && row.Provider == s.translator.Name() {
		provider = s.translator.DisplayName()
	}

	return &Result{Subject: row.Subject, Body: row.Body, SourceLang: lang, Provider: provider}
}

// call checks the budgets and translates src with one backend call. It
// returns the row to store, unsaved; userID is empty for background work,
// which counts against the site's budget only.
func (s *Service) call(ctx context.Context, src *source, userID string) (*core.Translation, error) {
	segs, err := splitSource(src)
	if err != nil {
		return nil, err
	}

	chars := translate.Chars(segs.texts...)

	if err := s.checkBudget(ctx, userID, chars); err != nil {
		return nil, err
	}

	out, err := s.translator.Translate(ctx, translate.Request{From: src.lang, To: TargetLang, Segments: segs.texts})
	if err != nil {
		return nil, err
	}

	subject, body, err := segs.apply(src, out)
	if err != nil {
		return nil, err
	}

	row := &core.Translation{
		SourceKind: src.kind,
		TargetLang: TargetLang,
		SourceHash: src.hash(),
		Provider:   s.translator.Name(),
		Subject:    subject,
		Body:       body,
		Chars:      chars,
	}

	if src.kind == core.TranslationSourceKindPost {
		row.PostID = null.StringFrom(src.id)
	} else {
		row.RSSItemID = null.StringFrom(src.id)
	}

	return row, nil
}

// miss makes the translation of src for a reader. Concurrent misses for one
// source in this process share one backend call, counted once.
func (s *Service) miss(ctx context.Context, src *source, userID string) (*core.Translation, error) {
	key := string(src.kind) + "/" + src.id + "/" + TargetLang

	v, err, _ := s.flights.Do(key, func() (any, error) {
		row, err := s.store.Translation(ctx, src.kind, src.id, TargetLang)
		if err != nil {
			return nil, err
		}

		if row != nil && row.SourceHash == src.hash() {
			return row, nil // made by a flight that just ended
		}

		row, err = s.call(ctx, src, userID)
		if err != nil {
			return nil, err
		}

		err = s.store.Tx(ctx, func(tx *repo.Store) error {
			if err := tx.AddTranslationUsage(ctx, userID, row.Provider, row.Chars); err != nil {
				return err
			}

			if err := tx.SaveTranslation(ctx, row); err != nil {
				return err
			}

			if src.kind == core.TranslationSourceKindPost {
				return tx.DeleteTranslationJob(ctx, src.id, TargetLang) // a failed job, done with
			}

			return nil
		})

		return row, err
	})
	if err != nil {
		return nil, err
	}

	return v.(*core.Translation), nil //nolint:forcetypeassert // the flight returns only rows
}

func (s *Service) checkBudget(ctx context.Context, userID string, chars int) error {
	if userID != "" {
		used, err := s.store.TranslationCharsToday(ctx, userID)
		if err != nil {
			return err
		}

		if used+chars > s.limits.UserDailyChars {
			return ErrLimit
		}
	}

	used, err := s.store.TranslationCharsThisMonth(ctx)
	if err != nil {
		return err
	}

	if used+chars > s.limits.SiteMonthlyChars {
		return ErrLimit
	}

	return nil
}

// segments is a source split for the backend: the subject first, if any,
// then the body's blocks.
type segments struct {
	hasSubject bool
	body       []markdown.Segment
	texts      []string
}

func splitSource(src *source) (*segments, error) {
	var (
		body []markdown.Segment
		err  error
	)

	if src.kind == core.TranslationSourceKindPost {
		body, err = markdown.Segments(src.body)
	} else {
		body, err = rss.Segments(src.body)
	}

	if err != nil {
		return nil, err
	}

	out := &segments{hasSubject: strings.TrimSpace(src.subject) != "", body: body}
	if out.hasSubject {
		out.texts = append(out.texts, markdown.TextSegment(src.subject))
	}

	for _, seg := range body {
		out.texts = append(out.texts, seg.Text)
	}

	return out, nil
}

func (segs *segments) apply(src *source, out []string) (subject, body string, err error) {
	if segs.hasSubject {
		if subject, err = markdown.PlainText(out[0]); err != nil {
			return "", "", err
		}

		out = out[1:]
	}

	if src.kind == core.TranslationSourceKindPost {
		body, err = markdown.Apply(src.body, segs.body, out)
	} else {
		body, err = rss.Apply(src.body, segs.body, out)
	}

	return subject, body, err
}

// Cached returns the cached, current translations of the sources ids the
// actor may read now, keyed by source ID, for pages that always translate
// some languages. It applies Translate's rules, never calls the backend and
// never counts; stale rows and sources without a control are left out.
func (s *Service) Cached(ctx context.Context, actor *core.User, kind core.TranslationSourceKind, ids []string) (map[string]*Result, error) {
	out := map[string]*Result{}
	if actor == nil || !s.Enabled() {
		return out, nil
	}

	var sources []*source

	switch kind {
	case core.TranslationSourceKindPost:
		rows, err := s.store.PostTranslations(ctx, ids, TargetLang)
		if err != nil {
			return nil, err
		}

		posts := make([]*core.Post, 0, len(rows))
		for _, row := range rows {
			posts = append(posts, row.R.Post)
		}

		if posts, err = s.reading.PostsToTranslate(ctx, actor, posts); err != nil {
			return nil, err
		}

		for _, p := range posts {
			if s.TranslatablePost(actor, p) {
				sources = append(sources, postSource(p))
			}
		}

		return s.fresh(rows, sources), nil
	case core.TranslationSourceKindRSSItem:
		rows, err := s.store.SubscribedRSSItemTranslations(ctx, actor.ID, ids, TargetLang)
		if err != nil {
			return nil, err
		}

		for _, row := range rows {
			if s.TranslatableRSSItem(actor, row.R.RSSItem.Language) {
				sources = append(sources, rssSource(row.R.RSSItem))
			}
		}

		return s.fresh(rows, sources), nil
	default:
		return out, nil
	}
}

// fresh keys the rows of the allowed sources whose hash still matches.
func (s *Service) fresh(rows core.TranslationSlice, allowed []*source) map[string]*Result {
	byID := make(map[string]*source, len(allowed))
	for _, src := range allowed {
		byID[src.id] = src
	}

	out := map[string]*Result{}

	for _, row := range rows {
		src := byID[row.PostID.String+row.RSSItemID.String]
		if src != nil && row.SourceHash == src.hash() {
			out[src.id] = s.result(src.lang, row)
		}
	}

	return out
}

// RetranslateStale queues re-translations of the post's cached translations
// on tx, after an edit. The provider isn't called here; until the worker
// re-makes a translation, Translate answers ErrUpdating. With translation
// off nothing is queued: the stale rows are re-made when it is back on.
func (s *Service) RetranslateStale(ctx context.Context, tx *repo.Store, postID string) error {
	if !s.Enabled() {
		return nil
	}

	langs, err := tx.PostTranslationLangs(ctx, postID)
	if err != nil {
		return err
	}

	for _, lang := range langs {
		if err := tx.QueueTranslationJob(ctx, postID, lang); err != nil {
			return err
		}
	}

	return nil
}

// Forget drops the post's cached translations and queued jobs on tx, such as
// when its author stops allowing translation.
func (s *Service) Forget(ctx context.Context, tx *repo.Store, postID string) error {
	return tx.DeletePostTranslations(ctx, postID)
}

var iso6391 = func() map[string]bool {
	codes := map[string]bool{}
	for l := range whatlanggo.Langs {
		if c := l.Iso6391(); c != "" {
			codes[c] = true
		}
	}

	return codes
}()

// Languages returns the source languages the actor always wants translated.
func (s *Service) Languages(ctx context.Context, actor *core.User) ([]string, error) {
	if actor == nil {
		return nil, service.ErrNeedsLogin
	}

	return s.store.TranslationLanguages(ctx, actor.ID)
}

// SetLanguages replaces the source languages the actor always wants
// translated: ISO 639-1 codes of languages pcom detects, English excluded.
func (s *Service) SetLanguages(ctx context.Context, actor *core.User, langs []string) error {
	if actor == nil {
		return service.ErrNeedsLogin
	}

	clean := make([]string, 0, len(langs))

	for _, l := range langs {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == TargetLang || !iso6391[l] {
			return service.Invalid("languages", fmt.Sprintf("%q is not a language that can be translated to English.", l))
		}

		clean = append(clean, l)
	}

	slices.Sort(clean)
	clean = slices.Compact(clean)

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		return tx.SetTranslationLanguages(ctx, actor.ID, clean)
	})
}

// Run sends the due re-translation jobs every interval until ctx is done.
func (s *Service) Run(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.runJobs(ctx); err != nil {
				slog.Warn("Failed to run translation jobs", "err", err.Error())
			}
		case <-ctx.Done():
			return
		}
	}
}

// claimLease is how long a claimed job is kept from other workers while
// the backend is called; a job neither done nor retried by then is taken
// again. siteLimitWait is how long a job waits when the site's budget is
// spent; that counts no attempt.
const (
	claimLease    = 10 * time.Minute
	siteLimitWait = time.Hour
	jobsPerRun    = 20
)

// runJobs sends the due jobs; a panic in a backend doesn't stop the worker.
// Jobs are claimed in one statement, the backend is called outside any
// transaction, and each job's result is stored in a transaction of its own,
// so one failure loses no other job's paid-for result.
func (s *Service) runJobs(ctx context.Context) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("translation jobs panicked: %v", p)
		}
	}()

	if !s.Enabled() {
		return nil
	}

	jobs, err := s.store.ClaimTranslationJobs(ctx, claimLease, jobsPerRun)
	if err != nil {
		return err
	}

	var errs []error

	for _, job := range jobs {
		if err := s.runJob(ctx, job); err != nil {
			errs = append(errs, fmt.Errorf("job %s of post %s: %w", job.ID, job.PostID, err))
		}
	}

	return errors.Join(errs...)
}

func retryWait(attempts int) time.Duration {
	if attempts < len(retryIntervals) {
		return retryIntervals[attempts]
	}

	return 0
}

// translatableNow reports whether a post still gets translations.
func translatableNow(p *core.Post) bool {
	return p.PublishedAt.Valid && p.AllowTranslation && translatableLang(p.Language)
}

// runJob re-makes one post's translation, counted against the site's
// budget only. A post that no longer gets translations drops its job, and
// over the site's budget the job waits without counting an attempt.
func (s *Service) runJob(ctx context.Context, job *core.TranslationJob) error {
	post, err := s.store.PostByID(ctx, job.PostID)
	if errors.Is(err, repo.ErrNotFound) {
		return s.store.DeleteTranslationJob(ctx, job.PostID, job.TargetLang)
	} else if err != nil {
		return err
	}

	if !translatableNow(post) {
		return s.store.DeleteTranslationJob(ctx, job.PostID, job.TargetLang)
	}

	src := postSource(post)

	row, err := s.call(ctx, src, "")
	if errors.Is(err, ErrLimit) {
		return s.store.PostponeTranslationJob(ctx, job.ID, siteLimitWait)
	} else if err != nil {
		if rerr := s.store.RetryTranslationJob(ctx, job.ID, retryWait(job.AttemptsNumber)); rerr != nil {
			return errors.Join(err, rerr)
		}

		return err
	}

	return s.store.Tx(ctx, func(tx *repo.Store) error {
		if err := tx.AddTranslationUsage(ctx, "", row.Provider, row.Chars); err != nil {
			return err
		}

		post, err := tx.PostByID(ctx, job.PostID)
		if errors.Is(err, repo.ErrNotFound) {
			return nil
		} else if err != nil {
			return err
		}

		switch {
		case !translatableNow(post):
			return tx.DeleteTranslationJob(ctx, job.PostID, job.TargetLang)
		case postSource(post).hash() != src.hash():
			return nil // edited again meanwhile: the job was queued anew
		}

		if err := tx.SaveTranslation(ctx, row); err != nil {
			return err
		}

		return tx.DeleteTranslationJob(ctx, job.PostID, job.TargetLang)
	})
}
