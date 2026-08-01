package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeFrontendPathRejectsTraversal(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"../secret", "../../secret", `..\\secret`} {
		if _, ok := safeFrontendPath(path); ok {
			t.Errorf("safeFrontendPath(%q) accepted traversal", path)
		}
	}
	if path, ok := safeFrontendPath("assets/app.js"); !ok || path != "assets/app.js" {
		t.Fatalf("safe asset path = %q, %v", path, ok)
	}
	if path, ok := safeFrontendPath(""); !ok || path != "index.html" {
		t.Fatalf("empty frontend path = %q, %v", path, ok)
	}
	if _, ok := safeFrontendPath("/etc/passwd"); ok {
		t.Fatal("absolute frontend path was accepted")
	}
}

func TestServeFrontendHandlesUnavailableUnsafeStaticAndFallbackPaths(t *testing.T) {
	t.Parallel()

	request := func(server *Server, requestPath string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, requestPath, nil)
		recorder := httptest.NewRecorder()
		server.serveFrontend(recorder, req)
		return recorder
	}

	unavailable := &Server{}
	if response := request(unavailable, "/"); response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "frontend_unavailable") {
		t.Fatalf("unavailable frontend status=%d body=%s", response.Code, response.Body.String())
	}
	if response := request(unavailable, "/api/missing"); response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "not_found") {
		t.Fatalf("API fallback status=%d body=%s", response.Code, response.Body.String())
	}

	directory := t.TempDir()
	server := &Server{options: Options{FrontendDir: directory}}
	unsafeRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	unsafeRequest.URL.Path = "/../secret"
	unsafeRecorder := httptest.NewRecorder()
	server.serveFrontend(unsafeRecorder, unsafeRequest)
	if unsafeRecorder.Code != http.StatusNotFound || !strings.Contains(unsafeRecorder.Body.String(), "not_found") {
		t.Fatalf("unsafe path status=%d body=%s", unsafeRecorder.Code, unsafeRecorder.Body.String())
	}
	if response := request(server, "/missing"); response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "frontend_unavailable") {
		t.Fatalf("missing index status=%d body=%s", response.Code, response.Body.String())
	}

	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("viki index"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "assets"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "assets", "app.js"), []byte("asset body"), 0o600); err != nil {
		t.Fatal(err)
	}
	if response := request(server, "/assets/app.js"); response.Code != http.StatusOK || response.Body.String() != "asset body" {
		t.Fatalf("asset status=%d body=%q", response.Code, response.Body.String())
	}
	if response := request(server, "/feature/client-signs"); response.Code != http.StatusOK || response.Body.String() != "viki index" {
		t.Fatalf("SPA fallback status=%d body=%q", response.Code, response.Body.String())
	}
}
