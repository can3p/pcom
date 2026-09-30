package posts

import (
	"bytes"
	"context"
	"io"

	"github.com/can3p/pcom/pkg/markdown"
	"github.com/can3p/pcom/pkg/media"
	"github.com/can3p/pcom/pkg/model/core"
	"github.com/can3p/pcom/pkg/postops"
	"github.com/can3p/pcom/pkg/repo"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/volatiletech/null/v8"
)

// ExportBlog is every post of the actor, with the images they embed, as a zip
// archive.
func (s *Service) ExportBlog(ctx context.Context, actor *core.User) ([]byte, error) {
	if err := requireActor(actor); err != nil {
		return nil, err
	}

	return s.export(ctx, actor.ID, "")
}

// ExportPost is one post of its author, with the images it embeds, as a zip
// archive. Whether the viewer may see the post is for the caller to decide.
func (s *Service) ExportPost(ctx context.Context, authorID, postID string) ([]byte, error) {
	return s.export(ctx, authorID, postID)
}

func (s *Service) export(ctx context.Context, authorID, postID string) ([]byte, error) {
	posts, err := s.store.PostsForExport(ctx, authorID, postID)
	if err != nil {
		return nil, err
	}

	return postops.SerializeBlogSlice(ctx, posts, s.storage)
}

// Import adds the posts and images of an exported archive to the actor's
// blog. A post that is the actor's own already is updated in place.
func (s *Service) Import(ctx context.Context, actor *core.User, archive []byte) (*postops.InjectStats, error) {
	if err := requireActor(actor); err != nil {
		return nil, err
	}

	posts, images, err := postops.DeserializeArchive(archive)
	if err != nil {
		return nil, err
	}

	return s.InjectPosts(ctx, actor, posts, images)
}

// InjectPosts stores parsed posts and images in the actor's blog, in one
// transaction. A post the actor owns already is updated in place, any other
// is created. An image the actor uploaded under that name before is not
// uploaded again.
func (s *Service) InjectPosts(ctx context.Context, actor *core.User, posts []*postops.PostWithMeta, images map[string][]byte) (*postops.InjectStats, error) {
	if err := requireActor(actor); err != nil {
		return nil, err
	}

	var stats *postops.InjectStats

	err := s.store.Tx(ctx, func(tx *repo.Store) error {
		var err error
		stats, err = s.inject(ctx, tx, actor.ID, posts, images)

		return err
	})
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func (s *Service) inject(ctx context.Context, tx *repo.Store, userID string, posts []*postops.PostWithMeta, images map[string][]byte) (*postops.InjectStats, error) {
	stats := &postops.InjectStats{}

	// current assumption: if you've guessed the name of the file in db, we assume
	// we don't need to reupload it
	// ideally we should do a checksum check ofc
	imgInDB, err := tx.UploadsByName(ctx, userID, lo.Keys(images))
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
		fname, err := s.storeImage(ctx, tx, userID, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}

		renameMap[name] = fname
		stats.ImagesUploaded++
	}

	postIDs := lo.Map(
		lo.Filter(posts, func(p *postops.PostWithMeta, idx int) bool { return p.Post.ID != "" }),
		func(p *postops.PostWithMeta, idx int) string { return p.Post.ID })

	existingPosts, err := tx.OwnPostsByIDs(ctx, userID, postIDs)
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

		body, err := markdown.ReplaceImageUrls(p.Body, markdown.ImportReplacer(renameMap, existingMap))
		if err != nil {
			return nil, err
		}

		p.Body = body
		p.UserID = userID

		if postWithMeta.Additional != nil && postWithMeta.Additional.URL != "" {
			url, err := tx.StoreURL(ctx, postWithMeta.Additional.URL)
			if err != nil {
				return nil, err
			}

			p.URLID = null.StringFrom(url.ID)
		}

		if insertPost {
			if err := tx.InsertPost(ctx, p); err != nil {
				return nil, err
			}

			stats.PostsCreated++
		} else {
			if err := tx.UpdatePost(ctx, p); err != nil {
				return nil, err
			}

			stats.PostsUpdated++
		}
	}

	return stats, nil
}

// UploadImage stores an image for the actor and returns its file name. The
// file lands in the media storage, so it is not part of a transaction.
func (s *Service) UploadImage(ctx context.Context, actor *core.User, r io.Reader) (string, error) {
	if err := requireActor(actor); err != nil {
		return "", err
	}

	return s.storeImage(ctx, s.store, actor.ID, r)
}

// storeImage records an image upload of a user and puts the file in the
// storage. The database row comes first, as media.HandleUpload does it.
func (s *Service) storeImage(ctx context.Context, store *repo.Store, userID string, r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	ftype := postops.DetectContentType(b)

	ext, err := media.ValidateImageType(ftype)
	if err != nil {
		return "", err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}

	fname := id.String() + ext

	upload := &core.MediaUpload{
		ID:            id.String(),
		UploadedFname: fname,
		ContentType:   ftype,
	}
	upload.UserID.SetValid(userID)

	if err := store.InsertUpload(ctx, upload); err != nil {
		return "", err
	}

	if err := s.storage.UploadFile(ctx, fname, b, ftype); err != nil {
		return "", err
	}

	return fname, nil
}
