package postops

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/media/server"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/types"
	"github.com/google/uuid"
	"github.com/volatiletech/sqlboiler/v4/boil"
	"github.com/volatiletech/sqlboiler/v4/queries/qm"
)

type ExportField string

const (
	OriginalID  ExportField = "original_id"
	Subject     ExportField = "subject"
	Visibility  ExportField = "visibility"
	PublishDate ExportField = "published"
)

func SerializePost(post *core.Post) []byte {
	var buf bytes.Buffer

	buf.WriteString("---\n")
	buf.WriteString(fmt.Sprintf("%s: %s\n", OriginalID, post.ID))
	if post.Subject.Valid {
		buf.WriteString(fmt.Sprintf("%s: %s\n", Subject, post.Subject.String))
	}
	buf.WriteString(fmt.Sprintf("%s: %s\n", Visibility, post.VisibilityRadius.String()))
	if post.PublishedAt.Valid {
		buf.WriteString(fmt.Sprintf("%s: %s\n", PublishDate, post.PublishedAt.Time.Format(time.RFC3339)))
	}
	buf.WriteString("---\n")

	buf.WriteString(fmt.Sprintf("\n%s", post.Body))

	return buf.Bytes()
}

func isURLMediaUpload(url string) bool {
	parts := strings.Split(url, ".")

	_, err := uuid.Parse(parts[0])

	return err == nil
}

func SerializeBlog(ctx context.Context, exec boil.ContextExecutor, mediaStorage server.MediaStorage, userID string, m ...qm.QueryMod) ([]byte, error) {
	mod := []qm.QueryMod{
		core.PostWhere.UserID.EQ(userID),
	}

	mod = append(mod, m...)

	posts, err := core.Posts(mod...).All(ctx, exec)

	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	defer func() {
		if err := w.Close(); err != nil {
			log.Printf("Error closing zip writer: %v", err)
		}
	}()

	imagesToDL := []*markdown.EmbeddedLink{}

	for _, p := range posts {
		mdPost := SerializePost(p)

		f, err := w.Create(p.ID + ".md")

		if err != nil {
			return nil, err
		}

		if _, err := f.Write(mdPost); err != nil {
			return nil, err
		}

		parsed := markdown.Parse(p.Body, types.ViewSinglePost, nil, nil)

		extracted := parsed.ExtractImageUrls()

		imagesToDL = append(imagesToDL, extracted...)
	}

	missingImages := []string{}

	for _, img := range imagesToDL {
		if !isURLMediaUpload(img.URL) {
			continue
		}

		r, _, _, err := mediaStorage.DownloadFile(ctx, img.URL)

		if err == media.ErrNotFound {
			missingImages = append(missingImages, img.URL)
			continue
		}

		if err != nil {
			return nil, err
		}

		b, err := io.ReadAll(r)

		if err != nil {
			return nil, err
		}

		f, err := w.Create(img.URL)

		if err != nil {
			return nil, err
		}

		if _, err := f.Write(b); err != nil {
			return nil, err
		}
	}

	if len(missingImages) > 0 {
		f, err := w.Create("missing_images.txt")

		if err != nil {
			return nil, err
		}

		text := strings.Join(missingImages, "\n")

		if _, err := f.Write([]byte(text)); err != nil {
			return nil, err
		}
	}

	if err := w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
