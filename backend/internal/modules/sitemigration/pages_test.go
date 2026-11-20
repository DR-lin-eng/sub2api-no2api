package sitemigration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPagesRejectsExistingNonEmptyDestination(t *testing.T) {
	data := t.TempDir()
	id := "3fd2db4e-7f0b-4da6-9852-bb79e5ef2b45"
	job := filepath.Join(data, "site-migration-clones", id)
	require.NoError(t, os.MkdirAll(filepath.Join(job, "pages"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(job, "pages", "old.md"), []byte("old"), 0600))
	p, err := newPageRestore(data, id)
	require.NoError(t, err)
	defer p.close()
	require.NoError(t, p.write(pagePart{Name: "new.md", Data: []byte("new"), End: true}))
	require.ErrorContains(t, p.publish(), "already exists")
}

func TestPagesRejectSymlinksAndEscapes(t *testing.T) {
	data := t.TempDir()
	id := "3fd2db4e-7f0b-4da6-9852-bb79e5ef2b45"
	outside := t.TempDir()
	job := filepath.Join(data, "site-migration-clones", id)
	require.NoError(t, os.MkdirAll(job, 0700))
	require.NoError(t, os.Symlink(outside, filepath.Join(job, "pages")))
	p, err := newPageRestore(data, id)
	require.NoError(t, err)
	defer p.close()
	require.NoError(t, p.write(pagePart{Name: "safe.md", Data: []byte("safe"), End: true}))
	require.Error(t, p.publish())
	require.Error(t, p.write(pagePart{Name: "../escape.md", Data: []byte("bad"), End: true}))
	require.Error(t, p.write(pagePart{Name: "bad\x00.md", Data: []byte("bad"), End: true}))
}

func TestPagesCrossChunkPublishRetryAndReceipt(t *testing.T) {
	data := t.TempDir()
	id := "3fd2db4e-7f0b-4da6-9852-bb79e5ef2b45"
	content := append([]byte(strings.Repeat("x", pageChunkSize)), []byte("tail")...)
	p, err := newPageRestore(data, id)
	require.NoError(t, err)
	require.NoError(t, p.write(pagePart{Name: "nested/page.md", Offset: 0, Data: content[:pageChunkSize]}))
	require.NoError(t, p.write(pagePart{Name: "nested/page.md", Offset: pageChunkSize, Data: content[pageChunkSize:], End: true}))
	require.NoError(t, p.publish())
	p.close()
	got, err := os.ReadFile(filepath.Join(data, "site-migration-clones", id, "pages", "nested", "page.md"))
	require.NoError(t, err)
	require.Equal(t, content, got)
	require.NoError(t, verifyPublishedPages(data, id))
	// A retry regenerates the same staged tree and accepts only the matching
	// immutable receipt and bytes.
	retry, err := newPageRestore(data, id)
	require.NoError(t, err)
	defer retry.close()
	require.NoError(t, retry.write(pagePart{Name: "nested/page.md", Offset: 0, Data: content[:pageChunkSize]}))
	require.NoError(t, retry.write(pagePart{Name: "nested/page.md", Offset: pageChunkSize, Data: content[pageChunkSize:], End: true}))
	require.NoError(t, retry.publish())
	require.NoError(t, verifyPublishedPages(data, id))
}
