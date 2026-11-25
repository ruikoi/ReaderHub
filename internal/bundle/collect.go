package bundle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	xhtml "golang.org/x/net/html"
	stdhtml "html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"readerhub/internal/document"
	"readerhub/internal/protocol"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type Bundle struct {
	Upload protocol.Upload
	Files  map[string][]byte
}

func Collect(ctx context.Context, path, title string, source document.Source) (Bundle, error) {
	var result Bundle
	absolute, err := filepath.Abs(path)
	if err != nil {
		return result, err
	}
	if strings.ToLower(filepath.Ext(absolute)) != ".md" && strings.ToLower(filepath.Ext(absolute)) != ".markdown" {
		return result, fmt.Errorf("first version accepts .md or .markdown files")
	}
	f, err := os.Open(absolute)
	if err != nil {
		return result, err
	}
	body, err := io.ReadAll(io.LimitReader(f, protocol.MaxMarkdownBytes+1))
	f.Close()
	if err != nil {
		return result, err
	}
	if len(body) == 0 || int64(len(body)) > protocol.MaxMarkdownBytes || !utf8.Valid(body) {
		return result, fmt.Errorf("article must be nonempty UTF-8 Markdown, up to 4 MiB")
	}
	source.Path = absolute
	result = Bundle{Upload: protocol.Upload{Title: title, Markdown: string(body), Source: source, Assets: []document.Asset{}}, Files: map[string][]byte{}}
	var total int64 = int64(len(body))
	for _, ref := range ImageReferences(body) {
		data, err := load(ctx, filepath.Dir(absolute), ref)
		if err != nil {
			return result, fmt.Errorf("image %q: %w", ref, err)
		}
		mime, ext, err := imageType(data)
		if err != nil {
			return result, fmt.Errorf("image %q: %w", ref, err)
		}
		if ext == "svg" {
			if err := checkSVG(data); err != nil {
				return result, fmt.Errorf("image %q: %w", ref, err)
			}
		}
		hash := sha256.Sum256(data)
		sum := hex.EncodeToString(hash[:])
		id := sum + "." + ext
		if _, exists := result.Files[id]; !exists {
			total += int64(len(data))
			if total > protocol.MaxUploadBytes-(2<<20) {
				return result, fmt.Errorf("article and images exceed the 64 MiB upload limit")
			}
			result.Files[id] = data
		}
		result.Upload.Assets = append(result.Upload.Assets, document.Asset{ID: id, Original: ref, SHA256: sum, MIME: mime, Size: int64(len(data))})
	}
	return result, nil
}

// ImageReferences uses the AST so reference images work and code examples are ignored.
func ImageReferences(body []byte) []string {
	root := goldmark.DefaultParser().Parse(text.NewReader(body))
	refs := []string{}
	seen := map[string]bool{}
	add := func(ref string) {
		ref = normalizeReference(ref)
		if ref != "" && !seen[ref] {
			refs = append(refs, ref)
			seen[ref] = true
		}
	}
	ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Image:
			add(string(node.Destination))
		case *ast.RawHTML:
			var b bytes.Buffer
			for i := 0; i < node.Segments.Len(); i++ {
				segment := node.Segments.At(i)
				b.Write(segment.Value(body))
			}
			htmlImages(b.Bytes(), add)
		case *ast.HTMLBlock:
			var b bytes.Buffer
			for i := 0; i < node.Lines().Len(); i++ {
				segment := node.Lines().At(i)
				b.Write(segment.Value(body))
			}
			if node.HasClosure() {
				b.Write(node.ClosureLine.Value(body))
			}
			htmlImages(b.Bytes(), add)
		}
		return ast.WalkContinue, nil
	})
	return refs
}

func htmlImages(b []byte, add func(string)) {
	t := xhtml.NewTokenizer(bytes.NewReader(b))
	for {
		kind := t.Next()
		if kind == xhtml.ErrorToken {
			return
		}
		if kind == xhtml.StartTagToken || kind == xhtml.SelfClosingTagToken {
			token := t.Token()
			if token.Data == "img" {
				for _, a := range token.Attr {
					if a.Key == "src" {
						add(a.Val)
					}
				}
			}
		}
	}
}

func load(ctx context.Context, dir, ref string) ([]byte, error) {
	if strings.HasPrefix(ref, "data:") {
		parts := strings.SplitN(ref, ",", 2)
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "data:image/") {
			return nil, fmt.Errorf("invalid data image")
		}
		var b []byte
		var err error
		if strings.HasSuffix(parts[0], ";base64") {
			b, err = base64.StdEncoding.DecodeString(parts[1])
		} else {
			var raw string
			raw, err = url.PathUnescape(parts[1])
			b = []byte(raw)
		}
		if int64(len(b)) > protocol.MaxImageBytes {
			return nil, fmt.Errorf("image exceeds 24 MiB")
		}
		return b, err
	}
	u, err := url.Parse(ref)
	if err != nil {
		return nil, err
	}
	var reader io.ReadCloser
	switch u.Scheme {
	case "http", "https":
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		response, err := (&http.Client{Timeout: 25 * time.Second}).Do(req)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != 200 {
			response.Body.Close()
			return nil, fmt.Errorf("remote image returned HTTP %d", response.StatusCode)
		}
		reader = response.Body
	case "", "file":
		if u.Host != "" && u.Host != "localhost" {
			return nil, fmt.Errorf("unsupported file host")
		}
		local := u.Path
		if !filepath.IsAbs(local) {
			local = filepath.Join(dir, local)
		}
		f, err := os.Open(local)
		if err != nil {
			return nil, err
		}
		reader = f
	default:
		return nil, fmt.Errorf("unsupported image scheme")
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, protocol.MaxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > protocol.MaxImageBytes {
		return nil, fmt.Errorf("image exceeds 24 MiB")
	}
	return data, nil
}

func imageType(data []byte) (string, string, error) {
	mime := http.DetectContentType(data)
	if bytes.Contains(bytes.TrimSpace(data), []byte("<svg")) {
		mime = "image/svg+xml"
	}
	if len(data) > 12 && string(data[4:8]) == "ftyp" && strings.Contains(string(data[8:min(len(data), 40)]), "avif") {
		mime = "image/avif"
	}
	ext := map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp", "image/svg+xml": "svg", "image/avif": "avif", "image/bmp": "bmp", "image/x-icon": "ico", "image/vnd.microsoft.icon": "ico"}[mime]
	if ext == "" {
		return "", "", fmt.Errorf("unsupported image format %s", mime)
	}
	return mime, ext, nil
}

var svgReference = regexp.MustCompile(`(?i)(?:href\s*=\s*["']([^"']+)["']|url\(\s*["']?([^)'"\s]+))`)

func checkSVG(data []byte) error {
	for _, match := range svgReference.FindAllSubmatch(data, -1) {
		ref := string(match[1])
		if ref == "" {
			ref = string(match[2])
		}
		if !strings.HasPrefix(ref, "#") && !strings.HasPrefix(ref, "data:") {
			return fmt.Errorf("SVG has an external dependency %q; embed it before submitting", ref)
		}
	}
	return nil
}

func normalizeReference(ref string) string {
	return stdhtml.UnescapeString(string(util.UnescapePunctuations([]byte(ref))))
}
