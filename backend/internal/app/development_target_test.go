package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type developmentTargetRoundTripper struct {
	response *http.Response
	err      error
}

func (transport developmentTargetRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return transport.response, transport.err
}

type failingDevelopmentTargetBody struct{}

var errReadDevelopmentTargetResponse = errors.New("read failed")

func (failingDevelopmentTargetBody) Read([]byte) (int, error) {
	return 0, errReadDevelopmentTargetResponse
}
func (failingDevelopmentTargetBody) Close() error { return nil }

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

func TestHTTPDevelopmentTargetRejectsBlankTokenBeforeRequest(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writeJSON(w, http.StatusOK, map[string]string{"receipt": "unexpected"})
	}))
	defer server.Close()

	_, err := newHTTPDevelopmentTarget(server.URL, "").Apply(context.Background(), "implementation")
	if err == nil {
		t.Fatal("development target accepted a blank token")
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("development target made %d request(s) without a token", got)
	}
}

func TestHTTPDevelopmentTargetRejectsRedirects(t *testing.T) {
	t.Parallel()
	var redirectedRequests atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirectedRequests.Add(1)
		writeJSON(w, http.StatusOK, map[string]string{"receipt": "unexpected"})
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL+"/implement", http.StatusFound)
	}))
	defer source.Close()

	_, err := newHTTPDevelopmentTarget(source.URL, "target-secret").Apply(context.Background(), "implementation")
	if err == nil {
		t.Fatal("development target accepted a redirect")
	}
	if got := redirectedRequests.Load(); got != 0 {
		t.Fatalf("development target followed redirect with %d request(s)", got)
	}
}

func TestHTTPDevelopmentTargetRejectsOversizedResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"receipt": strings.Repeat("x", 128*1024)})
	}))
	defer server.Close()

	_, err := newHTTPDevelopmentTarget(server.URL, "target-secret").Apply(context.Background(), "implementation")
	if err == nil {
		t.Fatal("development target accepted an oversized response")
	}
}

func TestHTTPDevelopmentTargetRejectsUnreadableResponse(t *testing.T) {
	t.Parallel()
	target := newHTTPDevelopmentTarget("http://target.test", "target-secret").(*httpDevelopmentTarget)
	target.client = &http.Client{Transport: developmentTargetRoundTripper{response: &http.Response{
		StatusCode: http.StatusOK,
		Body:       failingDevelopmentTargetBody{},
		Header:     make(http.Header),
	}}}

	if _, err := target.Apply(context.Background(), "implementation"); !errors.Is(err, errReadDevelopmentTargetResponse) {
		t.Fatalf("unreadable target response error=%v", err)
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
			token := "target-secret"
			if test.name == "not configured" {
				token = ""
			}
			_, err := newHTTPDevelopmentTarget(url, token).Apply(context.Background(), "implementation")
			if err == nil {
				t.Fatal("invalid target response was accepted")
			}
		})
	}

	server := httptest.NewServer(http.NotFoundHandler())
	target := newHTTPDevelopmentTarget(server.URL, "target-secret")
	server.Close()
	if _, err := target.Apply(context.Background(), strings.Repeat("x", 2)); err == nil {
		t.Fatal("unavailable target was accepted")
	}
}
