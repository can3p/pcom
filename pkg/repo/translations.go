package repo

import (
	"context"
	"time"

	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

// Times in translation_usage and translation_jobs are written and compared
// with the database's now(), never the Go clock, because the columns carry no
// time zone (#171).

// SubscribedRSSItem returns an RSS item of a feed userID is subscribed to,
// or ErrNotFound.
func (s *Store) SubscribedRSSItem(ctx context.Context, userID, itemID string) (*core.RSSItem, error) {
	item, err := core.RSSItems(
		core.RSSItemWhere.ID.EQ(itemID),
		qm.Where("exists (select 1 from user_feed_subscriptions ufs where ufs.feed_id = rss_items.feed_id and ufs.user_id = ?)", userID),
	).One(ctx, s.exec)

	return item, notFound(err)
}

func translationSourceCol(kind core.TranslationSourceKind) string {
	if kind == core.TranslationSourceKindRSSItem {
		return core.TranslationColumns.RSSItemID
	}

	return core.TranslationColumns.PostID
}

// Translation returns the cached translation of a source into lang, or nil.
func (s *Store) Translation(ctx context.Context, kind core.TranslationSourceKind, sourceID, lang string) (*core.Translation, error) {
	return orNil(core.Translations(
		core.TranslationWhere.SourceKind.EQ(kind),
		qm.Where(translationSourceCol(kind)+" = ?", sourceID),
		core.TranslationWhere.TargetLang.EQ(lang),
	).One(ctx, s.exec))
}

// PostTranslations returns the cached translations into lang of the posts
// ids, each with its post loaded (R.Post). Who may read them is the caller's
// rule.
func (s *Store) PostTranslations(ctx context.Context, ids []string, lang string) (core.TranslationSlice, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	return core.Translations(
		core.TranslationWhere.SourceKind.EQ(core.TranslationSourceKindPost),
		qm.WhereIn(core.TranslationColumns.PostID+" in ?", anys(ids)...),
		core.TranslationWhere.TargetLang.EQ(lang),
		qm.Load(core.TranslationRels.Post),
	).All(ctx, s.exec)
}

// SubscribedRSSItemTranslations returns the cached translations into lang of
// the RSS items ids that belong to a feed userID is subscribed to, each with
// its item loaded (R.RSSItem).
func (s *Store) SubscribedRSSItemTranslations(ctx context.Context, userID string, ids []string, lang string) (core.TranslationSlice, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	return core.Translations(
		core.TranslationWhere.SourceKind.EQ(core.TranslationSourceKindRSSItem),
		qm.WhereIn(core.TranslationColumns.RSSItemID+" in ?", anys(ids)...),
		core.TranslationWhere.TargetLang.EQ(lang),
		qm.Where(`exists (select 1 from rss_items ri join user_feed_subscriptions ufs on ufs.feed_id = ri.feed_id
			where ri.id = translations.rss_item_id and ufs.user_id = ?)`, userID),
		qm.Load(core.TranslationRels.RSSItem),
	).All(ctx, s.exec)
}

func anys(ids []string) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}

	return out
}

// SaveTranslation inserts t, or replaces the source's translation into the
// same language.
func (s *Store) SaveTranslation(ctx context.Context, t *core.Translation) error {
	if t.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}

		t.ID = id.String()
	}

	conflict := []string{translationSourceCol(t.SourceKind), core.TranslationColumns.TargetLang}

	return t.Upsert(ctx, s.exec, true, conflict, boil.Whitelist(
		core.TranslationColumns.SourceHash,
		core.TranslationColumns.Provider,
		core.TranslationColumns.Subject,
		core.TranslationColumns.Body,
		core.TranslationColumns.Chars,
		core.TranslationColumns.UpdatedAt,
	), boil.Infer())
}

// PostTranslationLangs returns the languages a post has cached translations into.
func (s *Store) PostTranslationLangs(ctx context.Context, postID string) ([]string, error) {
	rows, err := core.Translations(
		qm.Select(core.TranslationColumns.TargetLang),
		core.TranslationWhere.PostID.EQ(null.StringFrom(postID)),
	).All(ctx, s.exec)
	if err != nil {
		return nil, err
	}

	langs := make([]string, len(rows))
	for i, r := range rows {
		langs[i] = r.TargetLang
	}

	return langs, nil
}

// DeletePostTranslations deletes a post's cached translations and queued jobs.
func (s *Store) DeletePostTranslations(ctx context.Context, postID string) error {
	if _, err := core.Translations(core.TranslationWhere.PostID.EQ(null.StringFrom(postID))).DeleteAll(ctx, s.exec); err != nil {
		return err
	}

	_, err := core.TranslationJobs(core.TranslationJobWhere.PostID.EQ(postID)).DeleteAll(ctx, s.exec)

	return err
}

// QueueTranslationJob queues a re-translation of a post into lang, due now.
// A job already queued for the pair starts over.
func (s *Store) QueueTranslationJob(ctx context.Context, postID, lang string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	_, err = s.exec.ExecContext(ctx, `insert into translation_jobs (id, post_id, target_lang) values ($1, $2, $3)
		on conflict (post_id, target_lang) do update
		set status = 'new', attempts_number = 0, try_at = now(), updated_at = now()`, id.String(), postID, lang)

	return err
}

// PendingTranslationJob returns the post's queued (not failed) job for lang, or nil.
func (s *Store) PendingTranslationJob(ctx context.Context, postID, lang string) (*core.TranslationJob, error) {
	return orNil(core.TranslationJobs(
		core.TranslationJobWhere.PostID.EQ(postID),
		core.TranslationJobWhere.TargetLang.EQ(lang),
		core.TranslationJobWhere.Status.EQ(core.TranslationJobStatusNew),
	).One(ctx, s.exec))
}

// TranslationJobsOfPost returns every job of a post, failed ones included.
func (s *Store) TranslationJobsOfPost(ctx context.Context, postID string) (core.TranslationJobSlice, error) {
	return core.TranslationJobs(core.TranslationJobWhere.PostID.EQ(postID)).All(ctx, s.exec)
}

// ClaimTranslationJobs takes up to limit due jobs and pushes their try_at
// lease into the future in one statement, so that no other worker takes them
// while the backend is called outside any transaction. A job that is neither
// finished nor retried within the lease is taken again.
func (s *Store) ClaimTranslationJobs(ctx context.Context, lease time.Duration, limit int) (core.TranslationJobSlice, error) {
	var jobs core.TranslationJobSlice

	err := queries.Raw(`update translation_jobs set try_at = now() + make_interval(secs => $1), updated_at = now()
		where id in (select id from translation_jobs where status = 'new' and try_at <= now()
			order by try_at limit $2 for update skip locked)
		returning *`, lease.Seconds(), limit).Bind(ctx, s.exec, &jobs)

	return jobs, err
}

// PostponeTranslationJob makes a job due again after wait without counting an attempt.
func (s *Store) PostponeTranslationJob(ctx context.Context, jobID string, wait time.Duration) error {
	_, err := s.exec.ExecContext(ctx, `update translation_jobs set try_at = now() + make_interval(secs => $2), updated_at = now()
		where id = $1`, jobID, wait.Seconds())

	return err
}

// RetryTranslationJob records a failed attempt: the job is tried again after
// wait, or marked failed when wait is zero.
func (s *Store) RetryTranslationJob(ctx context.Context, jobID string, wait time.Duration) error {
	if wait == 0 {
		_, err := s.exec.ExecContext(ctx, `update translation_jobs set status = 'failed', updated_at = now() where id = $1`, jobID)
		return err
	}

	_, err := s.exec.ExecContext(ctx, `update translation_jobs
		set attempts_number = attempts_number + 1, try_at = now() + make_interval(secs => $2), updated_at = now()
		where id = $1`, jobID, wait.Seconds())

	return err
}

// DeleteTranslationJob deletes the post's job for lang, if any.
func (s *Store) DeleteTranslationJob(ctx context.Context, postID, lang string) error {
	_, err := core.TranslationJobs(
		core.TranslationJobWhere.PostID.EQ(postID),
		core.TranslationJobWhere.TargetLang.EQ(lang),
	).DeleteAll(ctx, s.exec)

	return err
}

// AddTranslationUsage records a backend call of chars characters; userID is
// empty for background work.
func (s *Store) AddTranslationUsage(ctx context.Context, userID, provider string, chars int) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}

	_, err = s.exec.ExecContext(ctx, `insert into translation_usage (id, user_id, provider, chars) values ($1, $2, $3, $4)`,
		id.String(), null.NewString(userID, userID != ""), provider, chars)

	return err
}

// TranslationCharsToday is what userID has had translated since the start of the day.
func (s *Store) TranslationCharsToday(ctx context.Context, userID string) (int, error) {
	return s.translationChars(ctx, `select coalesce(sum(chars), 0) from translation_usage
		where user_id = $1 and created_at >= date_trunc('day', now())`, userID)
}

// TranslationCharsThisMonth is what the whole site has had translated since
// the start of the month, background work included.
func (s *Store) TranslationCharsThisMonth(ctx context.Context) (int, error) {
	return s.translationChars(ctx, `select coalesce(sum(chars), 0) from translation_usage
		where created_at >= date_trunc('month', now())`)
}

func (s *Store) translationChars(ctx context.Context, query string, args ...any) (int, error) {
	var n int
	err := s.exec.QueryRowContext(ctx, query, args...).Scan(&n)

	return n, err
}

// TranslationLanguages returns the source languages a user always wants translated, sorted.
func (s *Store) TranslationLanguages(ctx context.Context, userID string) ([]string, error) {
	rows, err := core.UserTranslationLanguages(
		core.UserTranslationLanguageWhere.UserID.EQ(userID),
		qm.OrderBy(core.UserTranslationLanguageColumns.SourceLang),
	).All(ctx, s.exec)
	if err != nil {
		return nil, err
	}

	langs := make([]string, len(rows))
	for i, r := range rows {
		langs[i] = r.SourceLang
	}

	return langs, nil
}

// SetTranslationLanguages replaces a user's always-translate languages.
func (s *Store) SetTranslationLanguages(ctx context.Context, userID string, langs []string) error {
	if _, err := core.UserTranslationLanguages(core.UserTranslationLanguageWhere.UserID.EQ(userID)).DeleteAll(ctx, s.exec); err != nil {
		return err
	}

	for _, lang := range langs {
		row := &core.UserTranslationLanguage{UserID: userID, SourceLang: lang}
		if err := row.Insert(ctx, s.exec, boil.Infer()); err != nil {
			return err
		}
	}

	return nil
}
