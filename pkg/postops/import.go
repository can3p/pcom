package postops

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/volatiletech/null/v8"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

var headerRe = regexp.MustCompile(`^(\w+)\s*:\s*(.+)?$`)

func parseExportedPost(b []byte) (map[string]string, string, error) {
	lines := strings.Split(string(bytes.TrimSpace(b)), "\n")

	headers := map[string]string{}
	body := []string{}

	collectBody := false
	hasSeenDashes := false

	for idx, line := range lines {
		line = strings.TrimSpace(line)

		if idx == 0 && line == "---" {
			hasSeenDashes = true
			continue
		}

		if hasSeenDashes && line == "---" {
			collectBody = true
			continue
		}

		if collectBody {
			body = append(body, line)
			continue
		}

		if line == "" {
			continue
		}

		if matched := headerRe.FindStringSubmatch(line); matched != nil {
			headers[matched[1]] = strings.TrimSpace(matched[2])
		} else {
			body = append(body, line)
			collectBody = true
		}
	}

	return headers, strings.Trim(strings.Join(body, "\n"), "\n"), nil
}

type AdditionalFields struct {
	URL string
}

type PostWithMeta struct {
	Post       *core.Post
	Additional *AdditionalFields
}

func DeserializePost(b []byte) (*core.Post, *AdditionalFields, error) {
	headers, body, err := parseExportedPost(b)

	if err != nil {
		return nil, nil, err
	}

	post := &core.Post{
		Body: body,
	}

	var additionalFields *AdditionalFields

	for name, value := range headers {
		switch name {
		case string(OriginalID):
			_, err := uuid.Parse(value)

			if err != nil {
				return nil, nil, errors.Errorf("Invalid ID")
			}

			post.ID = value
		case string(Subject):
			post.Subject = null.NewString(value, value != "")
		case string(Url):
			additionalFields = &AdditionalFields{URL: value}
		case string(Visibility):
			vis := core.PostVisibility(value)

			if err := vis.IsValid(); err != nil {
				allVis := lo.Map(core.AllPostVisibility(), func(v core.PostVisibility, idx int) string { return v.String() })
				return nil, nil, errors.Errorf("Invalid visibility value, possible values are: %s", strings.Join(allVis, ", "))
			}

			post.VisibilityRadius = vis
		case string(PublishDate):
			d, err := time.Parse(time.RFC3339, value)

			if err != nil {
				return nil, nil, errors.Errorf("Invalid publish date")
			}

			post.PublishedAt = null.TimeFrom(d)
		default:
			return nil, nil, errors.Errorf("Unknown header: %s", name)
		}
	}

	return post, additionalFields, nil
}

func DeserializeArchive(b []byte) ([]*PostWithMeta, map[string][]byte, error) {
	r := bytes.NewReader(b)
	z, err := zip.NewReader(r, int64(len(b)))

	if err != nil {
		return nil, nil, err
	}

	var posts []*PostWithMeta
	images := make(map[string][]byte)

	for _, f := range z.File {
		fname := strings.ToLower(f.Name)

		// some mac os shit
		if strings.HasPrefix(fname, "__macosx") {
			continue
		}

		// the assumption is that we control the size of uploads in the general gin config
		// hence it's relatively safe to read all the contents
		// this code can still blow up, since big images can push the archive far above the limit
		rc, err := f.Open()

		if err != nil {
			return nil, nil, err
		}

		b, err := io.ReadAll(rc)

		if err != nil {
			return nil, nil, err
		}

		if err := rc.Close(); err != nil {
			return nil, nil, err
		}

		if strings.HasSuffix(fname, ".md") {
			post, additional, err := DeserializePost(b)

			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse file %s: %w", fname, err)
			}

			posts = append(posts, &PostWithMeta{
				Post:       post,
				Additional: additional,
			})
		}

		ftype := http.DetectContentType(b)

		if ftype == "image/png" || ftype == "image/jpeg" {
			images[fname] = b
		}
	}

	return posts, images, nil
}

type InjectStats struct {
	PostsCreated   int
	PostsUpdated   int
	ImagesUploaded int
	ImagesSkipped  int
}

func InjectPostsInDB(ctx context.Context, exec boil.ContextExecutor, mediaStorage server.MediaStorage, userID string, posts []*PostWithMeta, images map[string][]byte) (*InjectStats, error) {
	stats := &InjectStats{}

	// current assumption: if you've guessed the name of the file in db, we assume
	// we don't need to reupload it
	// ideally we should do a checksum check ofc
	imgInDB, err := core.MediaUploads(
		core.MediaUploadWhere.UploadedFname.IN(lo.Keys(images)),
		core.MediaUploadWhere.UserID.EQ(userID),
	).All(ctx, exec)

	if err != nil {
		return nil, err
	}

	// any new files should get a brand new name before upload
	renameMap := map[string]string{}
	existingMap := map[string]struct{}{}

	for _, img := range imgInDB {
		if _, ok := images[img.UploadedFname]; ok {
			stats.ImagesSkipped++
			delete(images, img.UploadedFname)
			existingMap[img.UploadedFname] = struct{}{}
		}
	}

	for name, b := range images {
		fname, err := media.HandleUpload(ctx, exec, mediaStorage, userID, bytes.NewReader(b))

		if err != nil {
			return nil, err
		}

		renameMap[name] = fname
		stats.ImagesUploaded++
	}

	postIDs := lo.Map(
		lo.Filter(posts, func(p *PostWithMeta, idx int) bool { return p.Post.ID != "" }),
		func(p *PostWithMeta, idx int) string { return p.Post.ID })

	existingPosts, err := core.Posts(
		core.PostWhere.UserID.EQ(userID),
		core.PostWhere.ID.IN(postIDs),
	).All(ctx, exec)

	if err != nil {
		return nil, err
	}

	keepIDs := map[string]struct{}{}

	for _, p := range existingPosts {
		keepIDs[p.ID] = struct{}{}
	}

	for _, postWithMeta := range posts {
		p := postWithMeta.Post

		_, keepPostID := keepIDs[p.ID]
		insertPost := p.ID == "" || !keepPostID

		if insertPost {
			id, err := uuid.NewV7()

			if err != nil {
				return nil, err
			}

			p.ID = id.String()
		}

		p.Body = markdown.ReplaceImageUrls(p.Body, markdown.ImportReplacer(renameMap, existingMap))
		p.UserID = userID

		if postWithMeta.Additional != nil && postWithMeta.Additional.URL != "" {
			url, err := StoreURL(ctx, exec, postWithMeta.Additional.URL)

			if err != nil {
				return nil, err
			}

			p.URLID = null.StringFrom(url.ID)
		}

		if insertPost {
			if err := p.Insert(ctx, exec, boil.Infer()); err != nil {
				return nil, err
			}
			stats.PostsCreated++
		} else {
			if _, err := p.Update(ctx, exec, boil.Infer()); err != nil {
				return nil, err
			}
			stats.PostsUpdated++
		}
	}

	return stats, nil
}
