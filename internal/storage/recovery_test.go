package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"readerhub/internal/document"
	"testing"
	"time"
)

func TestInterruptedImportAndDeletionRecover(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	body := "# A persisted article\n\nBody."
	sum := sha256.Sum256([]byte(body))
	fingerprint := hex.EncodeToString(sum[:])
	a := document.Article{ID: document.NewID(), Title: "A persisted article", ReadingState: "to_read", Queued: true, CreatedAt: time.Now(), UpdatedAt: time.Now(), BodySHA256: fingerprint, Assets: []document.Asset{}, Tags: []string{}}
	temp, err := os.MkdirTemp(filepath.Join(root, "incoming"), "test-")
	if err != nil {
		t.Fatal(err)
	}
	os.Mkdir(filepath.Join(temp, "assets"), 0700)
	a, _, err = s.Import(context.Background(), a, body, fingerprint, temp)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after directory commit but before the database record.
	if _, err = s.db.Exec("DELETE FROM articles WHERE id=?", a.ID); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	d, err := s.Get(context.Background(), a.ID)
	if err != nil || d.Markdown != body {
		t.Fatalf("orphan import not recovered: %v", err)
	}
	// Simulate a crash after moving files to trash but before deleting the row.
	if err = os.Rename(s.ArticleDir(a.ID), filepath.Join(root, "trash", a.ID)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(context.Background(), a.ID); err != nil {
		t.Fatal("interrupted deletion was not rolled back:", err)
	}
	// A committed deletion must not be resurrected by orphan recovery.
	os.Rename(s.ArticleDir(a.ID), filepath.Join(root, "trash", a.ID))
	s.db.Exec("DELETE FROM articles WHERE id=?", a.ID)
	s.Close()
	s, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	items, err := s.List(context.Background(), "queue", "")
	if err != nil || len(items) != 0 {
		t.Fatal("committed deletion resurrected")
	}
	if _, err = os.Stat(filepath.Join(root, "trash", a.ID)); !os.IsNotExist(err) {
		t.Fatal("committed trash was not cleaned")
	}
}
