package document

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type Source struct {
	DeviceID string `json:"device_id"`
	Machine  string `json:"machine"`
	Project  string `json:"project"`
	Path     string `json:"path"`
}

type Asset struct {
	ID       string `json:"id"`
	Original string `json:"original"`
	SHA256   string `json:"sha256"`
	MIME     string `json:"mime"`
	Size     int64  `json:"size"`
}

type Article struct {
	Fingerprint   string    `json:"fingerprint"`
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	OriginalTitle string    `json:"original_title"`
	Summary       string    `json:"summary"`
	Tags          []string  `json:"tags"`
	Source        Source    `json:"source"`
	Assets        []Asset   `json:"assets"`
	BodySHA256    string    `json:"body_sha256"`
	ReadingState  string    `json:"reading_state"`
	Saved         bool      `json:"saved"`
	Queued        bool      `json:"queued"`
	Progress      float64   `json:"progress"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	NoteCount     int       `json:"note_count"`
}

type Note struct {
	ID        string    `json:"id"`
	ArticleID string    `json:"article_id"`
	Quote     string    `json:"quote"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type Detail struct {
	Article
	Markdown string `json:"markdown"`
	Notes    []Note `json:"notes"`
}

func NewID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
