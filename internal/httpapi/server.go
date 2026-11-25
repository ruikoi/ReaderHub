package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"readerhub/internal/buildinfo"
	"readerhub/internal/hub"
	"readerhub/internal/protocol"
	"readerhub/internal/storage"
)

type Server struct {
	App   *hub.App
	Token string
	Web   fs.FS
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"status": "ok", "version": buildinfo.Version, "ai_enabled": false})
	})
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "readerhub_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		reply(w, 200, map[string]bool{"ok": true})
	})
	mux.Handle("/api/", s.auth(s.api()))
	if s.Web != nil {
		mux.Handle("/", http.FileServer(http.FS(s.Web)))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		if r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Origin") != "" {
			origin, err := url.Parse(r.Header.Get("Origin"))
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			if err != nil || origin.Host != r.Host || origin.Scheme != scheme {
				fail(w, 403, "跨来源请求被拒绝")
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if c, err := r.Cookie("readerhub_session"); err == nil && token == "" {
			token = c.Value
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(s.Token)) != 1 {
			fail(w, 401, "请先输入 Hub 访问口令")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &in, 1024) {
		return
	}
	if subtle.ConstantTimeCompare([]byte(in.Token), []byte(s.Token)) != 1 {
		fail(w, 401, "访问口令不正确")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "readerhub_session", Value: s.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil, MaxAge: 30 * 86400})
	reply(w, 200, map[string]bool{"ok": true})
}

func (s *Server) api() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"authenticated": true, "ai": map[string]any{"enabled": false, "status": "not_implemented"}})
	})
	m.HandleFunc("GET /api/items", func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("scope")
		if scope != "" && scope != "queue" && scope != "library" {
			fail(w, 400, "invalid scope")
			return
		}
		items, err := s.App.Store.List(r.Context(), scope, r.URL.Query().Get("q"))
		if err != nil {
			serverError(w, err)
			return
		}
		reply(w, 200, items)
	})
	m.HandleFunc("POST /api/submissions", s.upload)
	m.HandleFunc("GET /api/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		d, err := s.App.Store.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			storeError(w, err)
			return
		}
		reply(w, 200, d)
	})
	m.HandleFunc("PATCH /api/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		var p protocol.Patch
		if !decode(w, r, &p, 16<<10) {
			return
		}
		a, err := s.App.Store.Patch(r.Context(), r.PathValue("id"), p)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				storeError(w, err)
			} else {
				fail(w, 400, err.Error())
			}
			return
		}
		reply(w, 200, a)
	})
	m.HandleFunc("DELETE /api/items/{id}", func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("scope")
		if scope != "queue" && scope != "library" {
			fail(w, 400, "scope must be queue or library")
			return
		}
		if err := s.App.Store.Remove(r.Context(), r.PathValue("id"), scope); err != nil {
			storeError(w, err)
			return
		}
		reply(w, 200, map[string]bool{"ok": true})
	})
	m.HandleFunc("GET /api/items/{id}/assets/{asset}", s.asset)
	m.HandleFunc("GET /api/items/{id}/download", s.export)
	m.HandleFunc("POST /api/items/{id}/notes", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Quote string `json:"quote"`
			Text  string `json:"text"`
		}
		if !decode(w, r, &in, 128<<10) {
			return
		}
		n, err := s.App.Store.AddNote(r.Context(), r.PathValue("id"), in.Quote, in.Text)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				storeError(w, err)
			} else {
				fail(w, 400, err.Error())
			}
			return
		}
		reply(w, 201, n)
	})
	m.HandleFunc("PATCH /api/items/{id}/notes/{note}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Quote string `json:"quote"`
			Text  string `json:"text"`
		}
		if !decode(w, r, &in, 128<<10) {
			return
		}
		note, err := s.App.Store.UpdateNote(r.Context(), r.PathValue("id"), r.PathValue("note"), in.Quote, in.Text)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				storeError(w, err)
			} else {
				fail(w, 400, err.Error())
			}
			return
		}
		reply(w, 200, note)
	})
	m.HandleFunc("DELETE /api/items/{id}/notes/{note}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.App.Store.DeleteNote(r.Context(), r.PathValue("id"), r.PathValue("note")); err != nil {
			storeError(w, err)
			return
		}
		reply(w, 200, map[string]bool{"ok": true})
	})
	m.HandleFunc("POST /api/items/{id}/research", func(w http.ResponseWriter, r *http.Request) {
		fail(w, 501, "AI 研究整理尚未启用；你的想法已独立保存")
	})
	return m
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, protocol.MaxUploadBytes)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		if r.MultipartForm != nil {
			r.MultipartForm.RemoveAll()
		}
		fail(w, 400, "上传超过 64 MiB 或格式无效")
		return
	}
	defer r.MultipartForm.RemoveAll()
	var in protocol.Upload
	if len(r.FormValue("manifest")) > 12<<20 || json.Unmarshal([]byte(r.FormValue("manifest")), &in) != nil {
		fail(w, 400, "文章清单无效")
		return
	}
	a, duplicate, err := s.App.Accept(r.Context(), in, func(id string) (io.ReadCloser, error) {
		files := r.MultipartForm.File[id]
		if len(files) != 1 {
			return nil, os.ErrNotExist
		}
		return files[0].Open()
	})
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	status := 201
	if duplicate {
		status = 200
	}
	reply(w, status, protocol.Receipt{ID: a.ID, Title: a.Title, URL: "/#/article/" + a.ID, Duplicate: duplicate, Images: len(a.Assets)})
}

func (s *Server) asset(w http.ResponseWriter, r *http.Request) {
	d, err := s.App.Store.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		storeError(w, err)
		return
	}
	id := r.PathValue("asset")
	for _, a := range d.Assets {
		if a.ID == id {
			w.Header().Set("Content-Type", a.MIME)
			http.ServeFile(w, r, filepath.Join(s.App.Store.ArticleDir(d.ID), "assets", a.ID))
			return
		}
	}
	fail(w, 404, "图片不存在")
}

func decode(w http.ResponseWriter, r *http.Request, out any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		fail(w, 400, "请求格式无效")
		return false
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		fail(w, 400, "请求包含多余内容")
		return false
	}
	return true
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	reply(w, status, map[string]string{"error": msg})
}
func storeError(w http.ResponseWriter, err error) {
	if errors.Is(err, storage.ErrNotFound) {
		fail(w, 404, "文章或记录不存在")
	} else {
		serverError(w, err)
	}
}
func serverError(w http.ResponseWriter, err error) {
	log.Printf("ReaderHub storage error: %v", err)
	fail(w, 500, "服务暂时无法完成请求")
}

func HTTPServer(address string, handler http.Handler) *http.Server {
	return &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 2 * time.Minute, WriteTimeout: 2 * time.Minute, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
}
