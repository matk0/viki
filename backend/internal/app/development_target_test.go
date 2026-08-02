package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPDevelopmentTargetAppliesImplementation(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/implement" || request.Header.Get("Authorization") != "Bearer target-secret" {
			t.Fatalf("unexpected target request: path=%q authorization=%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["implementation"] != "contract signing" {
			t.Fatalf("implementation = %q", body["implementation"])
		}
		writeJSON(w, http.StatusOK, map[string]string{"receipt": " receipt-1 "})
	}))
	defer server.Close()

	target := newHTTPDevelopmentTarget(server.URL+"/", " target-secret ")
	receipt, err := target.Apply(context.Background(), "contract signing")
	if err != nil || receipt != "receipt-1" {
		t.Fatalf("receipt=%q error=%v", receipt, err)
	}
}

func TestHTTPDevelopmentTargetRejectsUnavailableOrInvalidResponses(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		handler http.HandlerFunc
		url     string
	}{
		{name: "not configured"},
		{name: "invalid URL", url: "://invalid"},
		{name: "status", handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }},
		{name: "invalid JSON", handler: func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not-json")) }},
		{name: "empty receipt", handler: func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusOK, map[string]string{"receipt": " "})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			url := test.url
			var server *httptest.Server
			if test.handler != nil {
				server = httptest.NewServer(test.handler)
				defer server.Close()
				url = server.URL
			}
			_, err := newHTTPDevelopmentTarget(url, "").Apply(context.Background(), "implementation")
			if err == nil {
				t.Fatal("invalid target response was accepted")
			}
		})
	}

	server := httptest.NewServer(http.NotFoundHandler())
	target := newHTTPDevelopmentTarget(server.URL, "")
	server.Close()
	if _, err := target.Apply(context.Background(), strings.Repeat("x", 2)); err == nil {
		t.Fatal("unavailable target was accepted")
	}
}
