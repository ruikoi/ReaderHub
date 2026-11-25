package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"readerhub/internal/bundle"
	"readerhub/internal/config"
	"readerhub/internal/document"
	"regexp"
)

var validDocumentID = regexp.MustCompile(`^[a-f0-9]{32}$`)

// Reconcile completes interrupted imports/deletes and refreshes portable mirrors
// from authoritative SQLite metadata. The Hub's data-directory lock excludes other processes.
func (s *Store) reconcile() error {
	rows, err := s.db.Query("SELECT id,metadata,search_text FROM articles")
	if err != nil {
		return err
	}
	type row struct{ id, metadata, body string }
	records := []row{}
	known := map[string]bool{}
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.metadata, &r.body); err != nil {
			rows.Close()
			return err
		}
		records = append(records, r)
		known[r.id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range records {
		dir := s.ArticleDir(r.id)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			if err := os.Rename(filepath.Join(s.Root, "trash", r.id), dir); err != nil {
				return fmt.Errorf("article files missing for %s: %w", r.id, err)
			}
		}
		var a document.Article
		if err := json.Unmarshal([]byte(r.metadata), &a); err != nil {
			return err
		}
		portable, err := bundle.PortableMarkdown([]byte(r.body), a.Assets)
		if err != nil {
			return err
		}
		for name, data := range map[string][]byte{"metadata.json": []byte(r.metadata), "source.md": []byte(r.body), "article.md": portable} {
			if err := config.AtomicWrite(filepath.Join(dir, name), data, 0600); err != nil {
				return err
			}
		}
	}
	entries, err := os.ReadDir(filepath.Join(s.Root, "articles"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || known[entry.Name()] || !validDocumentID.MatchString(entry.Name()) {
			continue
		}
		dir := s.ArticleDir(entry.Name())
		raw, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
		if err != nil {
			return err
		}
		var a document.Article
		if err = json.Unmarshal(raw, &a); err != nil {
			return err
		}
		if a.ID != entry.Name() || len(a.Fingerprint) != 64 {
			return fmt.Errorf("invalid orphan article metadata")
		}
		body, err := os.ReadFile(filepath.Join(dir, "source.md"))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != a.BodySHA256 {
			return fmt.Errorf("orphan article checksum mismatch")
		}
		for _, asset := range a.Assets {
			if filepath.Base(asset.ID) != asset.ID {
				return fmt.Errorf("invalid orphan image path")
			}
			b, err := os.ReadFile(filepath.Join(dir, "assets", asset.ID))
			if err != nil {
				return err
			}
			sum := sha256.Sum256(b)
			if hex.EncodeToString(sum[:]) != asset.SHA256 {
				return fmt.Errorf("orphan image checksum mismatch")
			}
		}
		if _, err = s.db.ExecContext(context.Background(), "INSERT INTO articles(id,fingerprint,metadata,search_text) VALUES(?,?,?,?)", a.ID, a.Fingerprint, string(raw), string(body)); err != nil {
			return err
		}
	}
	trash, err := os.ReadDir(filepath.Join(s.Root, "trash"))
	if err != nil {
		return err
	}
	for _, entry := range trash {
		if validDocumentID.MatchString(entry.Name()) && !known[entry.Name()] {
			if err = os.RemoveAll(filepath.Join(s.Root, "trash", entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
