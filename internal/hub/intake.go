package hub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"readerhub/internal/bundle"
	"readerhub/internal/document"
	"readerhub/internal/protocol"
	"readerhub/internal/storage"
)

var assetID = regexp.MustCompile(`^[a-f0-9]{64}\.(png|jpg|gif|webp|svg|avif|bmp|ico)$`)

type App struct{ Store *storage.Store }

func (a *App) Accept(ctx context.Context, u protocol.Upload, open func(string) (io.ReadCloser, error)) (document.Article, bool, error) {
	if strings.TrimSpace(u.Markdown) == "" || int64(len(u.Markdown)) > protocol.MaxMarkdownBytes || !utf8.ValidString(u.Markdown) {
		return document.Article{}, false, fmt.Errorf("article must be nonempty UTF-8 Markdown, up to 4 MiB")
	}
	if u.Source.DeviceID == "" || u.Source.Path == "" || len(u.Source.Path) > 4096 || len(u.Source.DeviceID) > 128 || len(u.Source.Project) > 200 || len(u.Source.Machine) > 200 || len(u.Assets) > 256 {
		return document.Article{}, false, fmt.Errorf("invalid source or too many images")
	}
	prepared, err := os.MkdirTemp(filepath.Join(a.Store.Root, "incoming"), "upload-*")
	if err != nil {
		return document.Article{}, false, err
	}
	defer os.RemoveAll(prepared)
	if err := os.Mkdir(filepath.Join(prepared, "assets"), 0700); err != nil {
		return document.Article{}, false, err
	}
	seen := map[string]bool{}
	origins := map[string]bool{}
	for _, asset := range u.Assets {
		if !assetID.MatchString(asset.ID) || asset.SHA256 != strings.Split(asset.ID, ".")[0] || asset.Size < 0 || asset.Size > protocol.MaxImageBytes || int64(len(asset.Original)) > protocol.MaxMarkdownBytes || asset.Original == "" || origins[asset.Original] {
			return document.Article{}, false, fmt.Errorf("invalid image manifest")
		}
		origins[asset.Original] = true
		if seen[asset.ID] {
			continue
		}
		seen[asset.ID] = true
		r, err := open(asset.ID)
		if err != nil {
			return document.Article{}, false, fmt.Errorf("missing image %s", asset.Original)
		}
		b, err := io.ReadAll(io.LimitReader(r, protocol.MaxImageBytes+1))
		r.Close()
		if err != nil {
			return document.Article{}, false, err
		}
		hash := sha256.Sum256(b)
		if int64(len(b)) != asset.Size || hex.EncodeToString(hash[:]) != asset.SHA256 {
			return document.Article{}, false, fmt.Errorf("image size or checksum mismatch: %s", asset.Original)
		}
		mime := http.DetectContentType(b)
		if strings.HasSuffix(asset.ID, ".svg") && strings.Contains(string(b), "<svg") {
			mime = "image/svg+xml"
		}
		if !strings.HasPrefix(mime, "image/") {
			return document.Article{}, false, fmt.Errorf("unsupported image content: %s", asset.Original)
		}
		if asset.MIME != mime && !(asset.MIME == "image/avif" && strings.HasSuffix(asset.ID, ".avif")) {
			return document.Article{}, false, fmt.Errorf("image media type mismatch")
		}
		if err = os.WriteFile(filepath.Join(prepared, "assets", asset.ID), b, 0600); err != nil {
			return document.Article{}, false, err
		}
	}
	bodyHash := sha256.Sum256([]byte(u.Markdown))
	assets := append([]document.Asset{}, u.Assets...)
	sort.Slice(assets, func(i, j int) bool { return assets[i].Original < assets[j].Original })
	fingerprintBytes, _ := json.Marshal(struct {
		Device string
		Path   string
		Body   string
		Assets []document.Asset
	}{u.Source.DeviceID, u.Source.Path, hex.EncodeToString(bodyHash[:]), assets})
	fingerprint := sha256.Sum256(fingerprintBytes)
	originalTitle, preview := bundle.Presentation([]byte(u.Markdown))
	title := strings.TrimSpace(u.Title)
	if title == "" {
		title = originalTitle
	}
	if title == "" {
		title = truncate(preview, 80)
	}
	if title == "" {
		title = filepath.Base(u.Source.Path)
	}
	title = truncate(title, 120)
	now := time.Now().UTC()
	article := document.Article{ID: document.NewID(), Title: title, OriginalTitle: originalTitle, Summary: truncate(preview, 180), Tags: []string{}, Source: u.Source, Assets: u.Assets, BodySHA256: hex.EncodeToString(bodyHash[:]), ReadingState: "to_read", Queued: true, CreatedAt: now, UpdatedAt: now}
	if article.Assets == nil {
		article.Assets = []document.Asset{}
	}
	return a.Store.Import(ctx, article, u.Markdown, hex.EncodeToString(fingerprint[:]), prepared)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
