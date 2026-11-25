package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"readerhub/internal/bundle"
	"readerhub/internal/config"
	"readerhub/internal/document"
	"readerhub/internal/protocol"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("article not found")

type Store struct {
	db   *sql.DB
	Root string
	mu   sync.Mutex
}

func Open(root string) (*Store, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	for _, dir := range []string{root, filepath.Join(root, "articles"), filepath.Join(root, "incoming"), filepath.Join(root, "trash")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "hub.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;")
	if err != nil {
		db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := os.Chmod(filepath.Join(root, "hub.db"), 0600); err != nil {
		db.Close()
		return nil, err
	}
	store := &Store{db: db, Root: root}
	if err := store.reconcile(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error                { return s.db.Close() }
func (s *Store) ArticleDir(id string) string { return filepath.Join(s.Root, "articles", id) }

func (s *Store) FindFingerprint(ctx context.Context, fingerprint string) (document.Article, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT metadata FROM articles WHERE fingerprint=?", fingerprint).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return document.Article{}, ErrNotFound
	}
	if err != nil {
		return document.Article{}, err
	}
	var a document.Article
	err = json.Unmarshal([]byte(raw), &a)
	return a, err
}

// Import commits a complete article bundle before exposing its database record.
// Failed database commits remove the newly placed bundle; existing documents are never replaced.
func (s *Store) Import(ctx context.Context, a document.Article, body, fingerprint, prepared string) (document.Article, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if old, err := s.FindFingerprint(ctx, fingerprint); err == nil {
		if !old.Queued {
			old.Queued = true
			old.UpdatedAt = time.Now().UTC()
			raw, _ := json.MarshalIndent(old, "", "  ")
			if _, err = s.db.ExecContext(ctx, "UPDATE articles SET metadata=? WHERE id=?", string(raw), old.ID); err != nil {
				return old, false, err
			}
			if err = config.AtomicWrite(filepath.Join(s.ArticleDir(old.ID), "metadata.json"), raw, 0600); err != nil {
				return old, false, err
			}
		}
		return old, true, nil
	} else if !errors.Is(err, ErrNotFound) {
		return a, false, err
	}
	a.Fingerprint = fingerprint
	raw, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return a, false, err
	}
	portable, err := bundle.PortableMarkdown([]byte(body), a.Assets)
	if err != nil {
		return a, false, err
	}
	if err = os.WriteFile(filepath.Join(prepared, "source.md"), []byte(body), 0600); err != nil {
		return a, false, err
	}
	if err = os.WriteFile(filepath.Join(prepared, "article.md"), portable, 0600); err != nil {
		return a, false, err
	}
	if err = os.WriteFile(filepath.Join(prepared, "metadata.json"), raw, 0600); err != nil {
		return a, false, err
	}
	final := s.ArticleDir(a.ID)
	if err = os.Rename(prepared, final); err != nil {
		return a, false, err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO articles(id,fingerprint,metadata,search_text) VALUES(?,?,?,?)", a.ID, fingerprint, string(raw), body)
	if err != nil {
		os.RemoveAll(final)
		return a, false, err
	}
	return a, false, nil
}

func (s *Store) Get(ctx context.Context, id string) (document.Detail, error) {
	var d document.Detail
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT metadata FROM articles WHERE id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}
	if err = json.Unmarshal([]byte(raw), &d.Article); err != nil {
		return d, err
	}
	b, err := os.ReadFile(filepath.Join(s.ArticleDir(id), "source.md"))
	if err != nil {
		return d, err
	}
	d.Markdown = string(b)
	d.Notes, err = s.Notes(ctx, id)
	d.NoteCount = len(d.Notes)
	return d, err
}

func (s *Store) List(ctx context.Context, scope, query string) ([]document.Article, error) {
	query = strings.TrimSpace(query)
	rows, err := s.db.QueryContext(ctx, `SELECT metadata, (SELECT count(*) FROM notes WHERE article_id=articles.id) FROM articles
	WHERE (?='' OR instr(lower(metadata),lower(?))>0 OR instr(lower(search_text),lower(?))>0 OR EXISTS(SELECT 1 FROM notes WHERE article_id=articles.id AND (instr(lower(text),lower(?))>0 OR instr(lower(quote),lower(?))>0))) ORDER BY json_extract(metadata,'$.created_at') DESC`, query, query, query, query, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []document.Article{}
	for rows.Next() {
		var raw string
		var count int
		if err := rows.Scan(&raw, &count); err != nil {
			return nil, err
		}
		var a document.Article
		if err := json.Unmarshal([]byte(raw), &a); err != nil {
			return nil, err
		}
		a.NoteCount = count
		if scope == "library" && !a.Saved {
			continue
		}
		if scope != "library" && !a.Queued {
			continue
		}
		items = append(items, a)
	}
	return items, rows.Err()
}

func (s *Store) Patch(ctx context.Context, id string, p protocol.Patch) (document.Article, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.Get(ctx, id)
	if err != nil {
		return d.Article, err
	}
	a := d.Article
	if p.Title != nil {
		a.Title = strings.TrimSpace(*p.Title)
		if a.Title == "" || len([]rune(a.Title)) > 200 {
			return a, fmt.Errorf("title must contain 1–200 characters")
		}
	}
	if p.Tags != nil {
		if len(*p.Tags) > 20 {
			return a, fmt.Errorf("too many tags")
		}
		a.Tags = []string{}
		seen := map[string]bool{}
		for _, t := range *p.Tags {
			t = strings.TrimSpace(t)
			if len([]rune(t)) > 40 {
				return a, fmt.Errorf("tag is too long")
			}
			if t != "" && !seen[strings.ToLower(t)] {
				a.Tags = append(a.Tags, t)
				seen[strings.ToLower(t)] = true
			}
		}
	}
	if p.ReadingState != nil {
		if *p.ReadingState != "to_read" && *p.ReadingState != "read" {
			return a, fmt.Errorf("invalid reading state")
		}
		a.ReadingState = *p.ReadingState
	}
	if p.Saved != nil {
		a.Saved = *p.Saved
	}
	if p.Queued != nil {
		a.Queued = *p.Queued
	}
	if p.Progress != nil {
		if *p.Progress < 0 || *p.Progress > 1 {
			return a, fmt.Errorf("invalid progress")
		}
		a.Progress = *p.Progress
	}
	if !a.Queued && !a.Saved {
		return a, fmt.Errorf("remove the article instead of clearing its final retention reference")
	}
	a.UpdatedAt = time.Now().UTC()
	raw, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return a, err
	}
	// SQLite is authoritative for mutable metadata; metadata.json is a portable mirror.
	if _, err = s.db.ExecContext(ctx, "UPDATE articles SET metadata=? WHERE id=?", string(raw), id); err != nil {
		return a, err
	}
	if err = config.AtomicWrite(filepath.Join(s.ArticleDir(id), "metadata.json"), raw, 0600); err != nil {
		return a, err
	}
	return a, nil
}

func (s *Store) Remove(ctx context.Context, id, scope string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	a := d.Article
	if scope == "library" {
		a.Saved = false
	} else {
		a.Queued = false
	}
	if a.Queued || a.Saved {
		a.UpdatedAt = time.Now().UTC()
		raw, _ := json.MarshalIndent(a, "", "  ")
		if _, err = s.db.ExecContext(ctx, "UPDATE articles SET metadata=? WHERE id=?", string(raw), id); err != nil {
			return err
		}
		return config.AtomicWrite(filepath.Join(s.ArticleDir(id), "metadata.json"), raw, 0600)
	}
	trash := filepath.Join(s.Root, "trash", id)
	if err = os.Rename(s.ArticleDir(id), trash); err != nil {
		return err
	}
	if _, err = s.db.ExecContext(ctx, "DELETE FROM articles WHERE id=?", id); err != nil {
		os.Rename(trash, s.ArticleDir(id))
		return err
	}
	return os.RemoveAll(trash)
}

func (s *Store) Notes(ctx context.Context, id string) ([]document.Note, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,quote,text,created_at FROM notes WHERE article_id=? ORDER BY created_at", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	notes := []document.Note{}
	for rows.Next() {
		var n document.Note
		var created string
		n.ArticleID = id
		if err := rows.Scan(&n.ID, &n.Quote, &n.Text, &created); err != nil {
			return nil, err
		}
		n.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

func (s *Store) AddNote(ctx context.Context, id, quote, text string) (document.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.Get(ctx, id); err != nil {
		return document.Note{}, err
	}
	if strings.TrimSpace(text) == "" || len([]rune(text)) > 10000 || len([]rune(quote)) > 20000 {
		return document.Note{}, fmt.Errorf("note is empty or too long")
	}
	n := document.Note{ID: document.NewID(), ArticleID: id, Quote: quote, Text: strings.TrimSpace(text), CreatedAt: time.Now().UTC()}
	_, err := s.db.ExecContext(ctx, "INSERT INTO notes(id,article_id,quote,text,created_at) VALUES(?,?,?,?,?)", n.ID, id, n.Quote, n.Text, n.CreatedAt.Format(time.RFC3339Nano))
	return n, err
}

func (s *Store) DeleteNote(ctx context.Context, id, noteID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.ExecContext(ctx, "DELETE FROM notes WHERE id=? AND article_id=?", noteID, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateNote(ctx context.Context, articleID, noteID, quote, text string) (document.Note, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(text) == "" || len([]rune(text)) > 10000 || len([]rune(quote)) > 20000 {
		return document.Note{}, fmt.Errorf("note is empty or too long")
	}
	var created string
	err := s.db.QueryRowContext(ctx, "SELECT created_at FROM notes WHERE id=? AND article_id=?", noteID, articleID).Scan(&created)
	if errors.Is(err, sql.ErrNoRows) {
		return document.Note{}, ErrNotFound
	}
	if err != nil {
		return document.Note{}, err
	}
	n := document.Note{ID: noteID, ArticleID: articleID, Quote: quote, Text: strings.TrimSpace(text)}
	n.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	_, err = s.db.ExecContext(ctx, "UPDATE notes SET quote=?,text=? WHERE id=? AND article_id=?", n.Quote, n.Text, noteID, articleID)
	return n, err
}
