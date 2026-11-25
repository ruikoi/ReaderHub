package httpapi

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	d, err := s.App.Store.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		storeError(w, err)
		return
	}
	names := []string{"article.md", "source.md"}
	seen := map[string]bool{}
	for _, a := range d.Assets {
		if !seen[a.ID] {
			names = append(names, "assets/"+a.ID)
			seen[a.ID] = true
		}
	}
	files := make([]*os.File, 0, len(names))
	for _, name := range names {
		f, err := os.Open(filepath.Join(s.App.Store.ArticleDir(d.ID), name))
		if err != nil {
			serverError(w, err)
			return
		}
		defer f.Close()
		files = append(files, f)
	}
	metadata, _ := json.MarshalIndent(d.Article, "", "  ")
	notes, _ := json.MarshalIndent(d.Notes, "", "  ")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="readerhub-`+d.ID+`.zip"`)
	archive := zip.NewWriter(w)
	for i, f := range files {
		entry, err := archive.Create(names[i])
		if err != nil {
			return
		}
		if _, err = io.Copy(entry, f); err != nil {
			return
		}
	}
	for name, data := range map[string][]byte{"metadata.json": metadata, "notes.json": notes} {
		entry, err := archive.Create(name)
		if err != nil {
			return
		}
		if _, err = entry.Write(data); err != nil {
			return
		}
	}
	archive.Close()
}
