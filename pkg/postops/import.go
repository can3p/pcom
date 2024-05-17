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
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/volatiletech/sqlboiler/v4/boil"
)

var headerRe = regexp.MustCompile(`^(\w+)\s*:\s*(.+)$`)

func parseExportedPost(b []byte) (map[string]string, string, error) {
	lines := strings.Split(string(b), "\n")

	headers := map[string]string{}
	body := []string{}

	collectBody := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if collectBody {
			body = append(body, line)
			continue
		}

		if line == "" {
			continue
		}

		if matched := headerRe.FindStringSubmatch(line); matched != nil {
			headers[matched[1]] = matched[2]
		} else {
			body = append(body, line)
			collectBody = true
		}
	}

	return headers, strings.Join(body, "\n"), nil
}

func DeserializePost(b []byte) (*core.Post, error) {
	headers, body, err := parseExportedPost(b)

	if err != nil {
		return nil, err
	}

	post := &core.Post{
		Body: body,
	}

	for name, value := range headers {
		switch name {
		case string(OriginalID):
			_, err := uuid.Parse(value)

			if err != nil {
				return nil, errors.Errorf("Invalid ID")
			}

			post.ID = value
		case string(Subject):
			post.Subject = value
		case string(Visibility):
			vis := core.PostVisibility(value)

			if err := vis.IsValid(); err != nil {
				allVis := lo.Map(core.AllPostVisibility(), func(v core.PostVisibility, idx int) string { return v.String() })
				return nil, errors.Errorf("Invalid visibility value, possible values are: %s", strings.Join(allVis, ", "))
			}

			post.VisbilityRadius = vis
		case string(PublishDate):
			d, err := time.Parse(time.RFC3339, value)

			if err != nil {
				return nil, errors.Wrapf(err, "invalid publish date")
			}

			post.PublishedAt = d
		default:
			return nil, errors.Errorf("Unknown header: %s", name)
		}
	}

	return post, nil
}

func DeserializeArchive(b []byte) ([]*core.Post, map[string][]byte, error) {

	r, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))

	if err != nil {
		return nil, nil, err
	}

	posts := []*core.Post{}
	images := map[string][]byte{}

	for _, f := range r.File {
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

		fname := strings.ToLower(f.Name)

		if strings.HasSuffix(fname, ".md") {
			p, err := DeserializePost(b)

			if err != nil {
				return nil, nil, err
			}

			posts = append(posts, p)
		}

		ftype := http.DetectContentType(b)

		fmt.Println(fname, ftype)

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

func InjectPostsInDB(ctx context.Context, exec boil.ContextExecutor, mediaServer media.MediaServer, userID string, posts []*core.Post, images map[string][]byte) (*InjectStats, error) {
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
		fname, err := media.HandleUpload(ctx, exec, mediaServer, userID, bytes.NewReader(b))

		if err != nil {
			return nil, err
		}

		renameMap[name] = fname
		stats.ImagesUploaded++
	}

	postIDs := lo.Map(
		lo.Filter(posts, func(p *core.Post, idx int) bool { return p.ID != "" }),
		func(p *core.Post, idx int) string { return p.ID })

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

	n := time.Now()

	for _, p := range posts {
		_, keepPostID := keepIDs[p.ID]
		insertPost := p.ID == "" || !keepPostID

		if insertPost {
			id, err := uuid.NewV7()

			if err != nil {
				return nil, err
			}

			p.ID = id.String()
			// change whenever drafts are introduced
			p.PublishedAt = n
		}

		p.Body = markdown.ReplaceImageUrls(p.Body, renameMap, existingMap)
		p.UserID = userID

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
