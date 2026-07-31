package app

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func (s *Server) serveFrontend(w http.ResponseWriter, request *http.Request) {
	if strings.HasPrefix(request.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "not_found", "Endpoint sa nenašiel.")
		return
	}
	if s.options.FrontendDir == "" {
		writeError(w, http.StatusNotFound, "frontend_unavailable", "Frontend ešte nie je zostavený.")
		return
	}
	cleanPath, safe := safeFrontendPath(strings.TrimPrefix(request.URL.Path, "/"))
	if !safe {
		writeError(w, http.StatusNotFound, "not_found", "Súbor sa nenašiel.")
		return
	}
	requestedFile := filepath.Join(s.options.FrontendDir, cleanPath)
	if info, err := os.Stat(requestedFile); err == nil && !info.IsDir() {
		http.ServeFile(w, request, requestedFile)
		return
	}
	index := filepath.Join(s.options.FrontendDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		writeError(w, http.StatusNotFound, "frontend_unavailable", "Frontend ešte nie je zostavený.")
		return
	}
	http.ServeFile(w, request, index)
}

func safeFrontendPath(raw string) (string, bool) {
	raw = strings.ReplaceAll(raw, `\`, "/")
	clean := path.Clean(raw)
	if clean == "." {
		return "index.html", true
	}
	if clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", false
	}
	return clean, true
}
