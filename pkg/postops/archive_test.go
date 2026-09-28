package postops_test

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/can3p/pcom/pkg/postops"
	"github.com/stretchr/testify/require"
)

func writeZipFile(t *testing.T, w *zip.Writer, name string, content []byte) {
	t.Helper()

	f, err := w.Create(name)
	require.NoError(t, err)

	_, err = f.Write(content)
	require.NoError(t, err)
}

func TestDeserializeArchive_SkipsMacOSXEntries(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	writeZipFile(t, w, "__MACOSX/._post.md", []byte("garbage"))
	require.NoError(t, w.Close())

	posts, images, err := postops.DeserializeArchive(buf.Bytes())
	require.NoError(t, err)
	require.Empty(t, posts)
	require.Empty(t, images)
}

func TestDeserializeArchive_UnparseablePostNamesTheFile(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	writeZipFile(t, w, "broken.md", []byte("---\nunknown_header: yes\n---\n\nbody"))
	require.NoError(t, w.Close())

	_, _, err := postops.DeserializeArchive(buf.Bytes())
	require.Error(t, err)
	require.Contains(t, err.Error(), "broken.md")
}

func TestDeserializeArchive_NonMdNonImageFileIsIgnored(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	writeZipFile(t, w, "readme.txt", []byte("just text, nothing else"))
	require.NoError(t, w.Close())

	posts, images, err := postops.DeserializeArchive(buf.Bytes())
	require.NoError(t, err)
	require.Empty(t, posts)
	require.Empty(t, images)
}

func TestDeserializeArchive_ImageNamesAreLowercased(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	png := []byte("\x89PNG\r\n\x1a\nrest-of-file")
	writeZipFile(t, w, "PHOTO.PNG", png)
	require.NoError(t, w.Close())

	_, images, err := postops.DeserializeArchive(buf.Bytes())
	require.NoError(t, err)
	require.Contains(t, images, "photo.png")
	require.Equal(t, png, images["photo.png"])
}

func TestDeserializeArchive_NotAZipFile(t *testing.T) {
	t.Parallel()

	_, _, err := postops.DeserializeArchive([]byte("definitely not a zip file"))
	require.Error(t, err)
}

func TestDeserializeArchive_EmptyArchive(t *testing.T) {
	t.Parallel()

	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	require.NoError(t, w.Close())

	posts, images, err := postops.DeserializeArchive(buf.Bytes())
	require.NoError(t, err)
	require.Empty(t, posts)
	require.Empty(t, images)
}
