package httpapi_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"readerhub/internal/bundle"
	"readerhub/internal/client"
	"readerhub/internal/document"
	"readerhub/internal/httpapi"
	"readerhub/internal/hub"
	"readerhub/internal/storage"
	"strings"
	"sync"
	"testing"
)

const token = "test-token-that-is-not-a-real-credential"

func start(t *testing.T, root string) (*storage.Store, *httptest.Server) {
	t.Helper()
	s, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer((&httpapi.Server{App: &hub.App{Store: s}, Token: token}).Handler())
	t.Cleanup(func() { server.Close(); s.Close() })
	return s, server
}
func request(t *testing.T, url, method, path string, body any, auth bool) (int, []byte) {
	t.Helper()
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	r, _ := http.NewRequest(method, url+path, bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	if auth {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}
func fixture(t *testing.T) bundle.Bundle {
	t.Helper()
	dir := t.TempDir()
	var pngData bytes.Buffer
	png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	os.WriteFile(filepath.Join(dir, "a.png"), pngData.Bytes(), 0600)
	os.WriteFile(filepath.Join(dir, "article.md"), []byte("# 并发与内存屏障\n\n一篇适合在平板阅读的文章。\n\n![image](a.png)"), 0600)
	b, err := bundle.Collect(context.Background(), filepath.Join(dir, "article.md"), "", document.Source{DeviceID: "source-one", Machine: "测试电脑", Project: "系统研究"})
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestEndToEndUploadNotesManualStatusAndRetention(t *testing.T) {
	root := t.TempDir()
	_, server := start(t, root)
	b := fixture(t)
	c := client.Client{URL: server.URL, Token: token}
	receipt, err := c.Submit(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	again, err := c.Submit(context.Background(), b)
	if err != nil || !again.Duplicate || again.ID != receipt.ID {
		t.Fatalf("retry was not idempotent: %+v %v", again, err)
	}
	path := "/api/items/" + receipt.ID
	status, _ := request(t, server.URL, "GET", path, nil, false)
	if status != 401 {
		t.Fatalf("unauthenticated read: %d", status)
	}
	status, raw := request(t, server.URL, "GET", path, nil, true)
	var d document.Detail
	json.Unmarshal(raw, &d)
	if status != 200 || d.ReadingState != "to_read" || len(d.Assets) != 1 {
		t.Fatalf("bad detail %d %s", status, raw)
	}
	// Remove the original files: all subsequent reads must be served by the Hub.
	os.RemoveAll(filepath.Dir(b.Upload.Source.Path))
	req, _ := http.NewRequest("GET", server.URL+path+"/assets/"+d.Assets[0].ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	imageResponse, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	copied, _ := io.ReadAll(imageResponse.Body)
	imageResponse.Body.Close()
	if imageResponse.StatusCode != 200 || !bytes.Equal(copied, b.Files[d.Assets[0].ID]) {
		t.Fatal("Hub image depends on source")
	}
	status, noteRaw := request(t, server.URL, "POST", path+"/notes", map[string]string{"quote": "内存屏障", "text": "之后在共享缓冲区排查时再看"}, true)
	if status != 201 {
		t.Fatalf("note: %d", status)
	}
	var savedNote document.Note
	json.Unmarshal(noteRaw, &savedNote)
	status, _ = request(t, server.URL, "PATCH", path+"/notes/"+savedNote.ID, map[string]string{"quote": "内存屏障", "text": "之后在共享缓冲区排查时再看（已补充）"}, true)
	if status != 200 {
		t.Fatal("editing a note failed", status)
	}
	status, _ = request(t, server.URL, "PATCH", path, map[string]any{"saved": true, "progress": 1, "tags": []string{"Linux", "并发"}}, true)
	if status != 200 {
		t.Fatalf("save: %d", status)
	}
	_, raw = request(t, server.URL, "GET", path, nil, true)
	json.Unmarshal(raw, &d)
	if d.ReadingState != "to_read" || !d.Saved || len(d.Notes) != 1 {
		t.Fatal("opening, notes or saving changed manual status")
	}
	_, raw = request(t, server.URL, "GET", "/api/items?scope=library&q=共享缓冲区", nil, true)
	var items []document.Article
	json.Unmarshal(raw, &items)
	if len(items) != 1 {
		t.Fatalf("note search failed: %s", raw)
	}
	status, _ = request(t, server.URL, "POST", path+"/research", nil, true)
	if status != 501 {
		t.Fatalf("AI must remain disabled: %d", status)
	}
	status, _ = request(t, server.URL, "DELETE", path+"?scope=queue", nil, true)
	if status != 200 {
		t.Fatal(status)
	}
	_, raw = request(t, server.URL, "GET", path, nil, true)
	json.Unmarshal(raw, &d)
	if d.Queued || !d.Saved || len(d.Markdown) == 0 {
		t.Fatal("removing queue entry removed saved content")
	}
	status, _ = request(t, server.URL, "PATCH", path, map[string]string{"reading_state": "read"}, true)
	if status != 200 {
		t.Fatal(status)
	}
	// Export is a standalone Markdown/image package and includes user notes.
	status, archiveBytes := request(t, server.URL, "GET", path+"/download", nil, true)
	if status != 200 {
		t.Fatal(status)
	}
	archive, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		t.Fatal(err)
	}
	foundBody, foundImage, foundNotes := false, false, false
	for _, file := range archive.File {
		reader, _ := file.Open()
		data, _ := io.ReadAll(reader)
		reader.Close()
		switch file.Name {
		case "article.md":
			foundBody = bytes.Contains(data, []byte("./assets/"))
		case "notes.json":
			foundNotes = bytes.Contains(data, []byte("共享缓冲区"))
		}
		if strings.HasPrefix(file.Name, "assets/") {
			foundImage = true
		}
	}
	if !foundBody || !foundImage || !foundNotes {
		t.Fatal("incomplete standalone export")
	}
	// A separate connection sees persisted content, metadata and notes.
	reopened, err := storage.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := reopened.Get(context.Background(), receipt.ID)
	if err != nil || restored.ReadingState != "read" || len(restored.Notes) != 1 {
		t.Fatalf("persistence: %+v %v", restored, err)
	}
	status, _ = request(t, server.URL, "DELETE", path+"?scope=library", nil, true)
	if status != 200 {
		t.Fatal(status)
	}
	status, _ = request(t, server.URL, "GET", path, nil, true)
	if status != 404 {
		t.Fatal("unreferenced content was not removed")
	}
}
func TestRejectBadChecksumBeforeVisibleAndConcurrentDedup(t *testing.T) {
	s, server := start(t, t.TempDir())
	b := fixture(t)
	bad := b
	bad.Upload.Assets = append([]document.Asset{}, b.Upload.Assets...)
	bad.Upload.Assets[0].Size++
	if _, err := (client.Client{URL: server.URL, Token: token}).Submit(context.Background(), bad); err == nil {
		t.Fatal("invalid manifest accepted")
	}
	list, _ := s.List(context.Background(), "queue", "")
	if len(list) != 0 {
		t.Fatal("incomplete article became visible")
	}
	var wg sync.WaitGroup
	ids := make(chan string, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := (client.Client{URL: server.URL, Token: token}).Submit(context.Background(), b)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- r.ID
		}()
	}
	wg.Wait()
	close(ids)
	one := ""
	for id := range ids {
		if one != "" && id != one {
			t.Fatal("concurrent duplicate records")
		}
		one = id
	}
}
func TestBrowserLoginAndCrossOriginProtection(t *testing.T) {
	_, server := start(t, t.TempDir())
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar}
	login, _ := http.NewRequest("POST", server.URL+"/api/login", bytes.NewBufferString(`{"token":"`+token+`"}`))
	login.Header.Set("Content-Type", "application/json")
	response, err := browser.Do(login)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	response, err = browser.Get(server.URL + "/api/session")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("browser session did not persist")
	}
	malicious, _ := http.NewRequest("POST", server.URL+"/api/logout", nil)
	malicious.Header.Set("Origin", "http://untrusted.example")
	response, err = browser.Do(malicious)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("cross-origin mutation was accepted")
	}
}
