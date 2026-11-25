package bundle

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"readerhub/internal/document"
	"reflect"
	"testing"
)

func pngBytes(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, c)
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}
func TestImageReferencesIgnoreCodeAndResolveReferences(t *testing.T) {
	body := []byte("# Example\n![one](../images/a.png)\n![two][photo]\n\n[photo]: <./images/b c.png>\n\n<img src=\"./images/html.svg\">\n\n`![ignored](nope.png)`\n\n```md\n![ignored](missing.png)\n```\n")
	want := []string{"../images/a.png", "./images/b c.png", "./images/html.svg"}
	if got := ImageReferences(body); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}
func TestCollectPreservesSourceAndHandlesSameNames(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs")
	images := filepath.Join(root, "images")
	os.MkdirAll(docs, 0700)
	os.MkdirAll(images, 0700)
	first := pngBytes(color.RGBA{R: 255, A: 255})
	second := pngBytes(color.RGBA{B: 255, A: 255})
	os.WriteFile(filepath.Join(docs, "image.png"), first, 0600)
	os.WriteFile(filepath.Join(images, "image.png"), second, 0600)
	source := []byte("An untitled document.\n\n![a](image.png)\n![b](../images/image.png)\n![again](./image.png)\n")
	file := filepath.Join(docs, "article.md")
	os.WriteFile(file, source, 0600)
	b, err := Collect(context.Background(), file, "", document.Source{DeviceID: "example"})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Files) != 2 || len(b.Upload.Assets) != 3 {
		t.Fatalf("files=%d refs=%d", len(b.Files), len(b.Upload.Assets))
	}
	if b.Upload.Assets[0].ID == b.Upload.Assets[1].ID {
		t.Fatal("different files with same names collided")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(after, source) || b.Upload.Markdown != string(source) {
		t.Fatal("source was changed")
	}
	os.Remove(filepath.Join(docs, "image.png"))
	if _, err = Collect(context.Background(), file, "", document.Source{}); err == nil {
		t.Fatal("missing images must fail before upload")
	}
}
func TestNetworkAndInlineImagesBecomeResources(t *testing.T) {
	data := pngBytes(color.RGBA{G: 255, A: 255})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(data)
	}))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "article.md")
	os.WriteFile(file, []byte("![network]("+server.URL+"/image.png)\n![inline](data:image/png;base64,"+base64.StdEncoding.EncodeToString(data)+")"), 0600)
	b, err := Collect(context.Background(), file, "", document.Source{})
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Upload.Assets) != 2 || len(b.Files) != 1 {
		t.Fatalf("expected two refs deduplicated into one file: %+v", b.Upload.Assets)
	}
}
func TestSVGExternalDependencyIsNotSilentlyLost(t *testing.T) {
	if err := checkSVG([]byte(`<svg><image href="outside.png"/></svg>`)); err == nil {
		t.Fatal("external dependency accepted")
	}
	if err := checkSVG([]byte(`<svg><use href="#shape"/></svg>`)); err != nil {
		t.Fatal(err)
	}
}

func TestPortableMarkdownPreservesCodeAndProse(t *testing.T) {
	body := []byte("Text mentions a.png unchanged.\n\n![one](a.png)\n![two][ref]\n\n[ref]: a.png\n\n<img src=\"a.png\" alt=\"a\">\n\n`![code](a.png)`\n\n```md\n![code](a.png)\n```\n")
	portable, err := PortableMarkdown(body, []document.Asset{{Original: "a.png", ID: "example.png"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"./assets/example.png"}
	if refs := ImageReferences(portable); !reflect.DeepEqual(refs, want) {
		t.Fatalf("portable refs %v\n%s", refs, portable)
	}
	if !bytes.Contains(portable, []byte("Text mentions a.png unchanged.")) || !bytes.Contains(portable, []byte("`![code](a.png)`")) || !bytes.Contains(portable, []byte("```md\n![code](a.png)")) {
		t.Fatalf("non-image content was rewritten:\n%s", portable)
	}
}

func TestPresentationUsesContentNotCodeOrFileName(t *testing.T) {
	title, preview := Presentation([]byte("---\ntitle: internal metadata\n---\n\nA useful explanation of memory barriers.\n\n```sh\n# not a title\n```\n"))
	if title != "" || preview != "A useful explanation of memory barriers." {
		t.Fatalf("incorrect untitled presentation: %q %q", title, preview)
	}
	title, preview = Presentation([]byte("# A **real** title\n\n正文中的知识点。\n\n![image](pic.png)\n"))
	if title != "A real title" || preview != "正文中的知识点。" {
		t.Fatalf("incorrect presentation: %q %q", title, preview)
	}
}

func TestPortableMarkdownQuotedAndAngleDestinations(t *testing.T) {
	for _, input := range []string{"![x](john's.png)", "![x](<a)b.png>)", "![x](a.png 'a (title)')"} {
		refs := ImageReferences([]byte(input))
		if len(refs) != 1 {
			t.Fatalf("test input did not contain an image: %s", input)
		}
		out, err := PortableMarkdown([]byte(input), []document.Asset{{Original: refs[0], ID: "test.png"}})
		if err != nil {
			t.Fatalf("%s: %v", input, err)
		}
		if got := ImageReferences(out); len(got) != 1 || got[0] != "./assets/test.png" {
			t.Fatalf("incorrect rewrite %s -> %s", input, out)
		}
	}
}
