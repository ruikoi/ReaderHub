package protocol

import "readerhub/internal/document"

const MaxUploadBytes int64 = 64 << 20
const MaxImageBytes int64 = 24 << 20
const MaxMarkdownBytes int64 = 4 << 20

type Upload struct {
	Title    string           `json:"title"`
	Markdown string           `json:"markdown"`
	Source   document.Source  `json:"source"`
	Assets   []document.Asset `json:"assets"`
}

type Receipt struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Duplicate bool   `json:"duplicate"`
	Images    int    `json:"images"`
}

type Patch struct {
	Title        *string   `json:"title,omitempty"`
	Tags         *[]string `json:"tags,omitempty"`
	ReadingState *string   `json:"reading_state,omitempty"`
	Saved        *bool     `json:"saved,omitempty"`
	Queued       *bool     `json:"queued,omitempty"`
	Progress     *float64  `json:"progress,omitempty"`
}
