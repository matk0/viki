package localize

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sourceContent() Content {
	return Content{Title: "Price per 100 grams", BodyMD: "See `VOZ-SC-PRICING-001` at https://example.test/spec and {quantity}.", StepTexts: []string{"the customer orders 2 pieces"}}
}
func slovakContent() Content {
	return Content{Title: "Cena za 100 gramov", BodyMD: "Pozri `VOZ-SC-PRICING-001` na https://example.test/spec a {quantity}.", StepTexts: []string{"zákazník objedná 2 kusy"}}
}
func TestTranslationPreservesProtectedBusinessValues(t *testing.T) {
	source, target := sourceContent(), slovakContent()
	if err := Validate(source, target); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Content){
		func(c *Content) { c.Title = "" }, func(c *Content) { c.Title = strings.Repeat("a", 1001) }, func(c *Content) { c.BodyMD = strings.Repeat("a", 200001) },
		func(c *Content) { c.StepTexts = nil }, func(c *Content) { c.StepTexts = []string{} }, func(c *Content) { c.StepTexts = []string{""} },
		func(c *Content) { c.StepTexts = []string{"zákazník objedná 200 kusov"} }, func(c *Content) {
			c.BodyMD = strings.ReplaceAll(c.BodyMD, "https://example.test/spec", "https://wrong.test")
		},
	} {
		bad := slovakContent()
		change(&bad)
		if Validate(source, bad) == nil {
			t.Fatalf("accepted altered translation: %+v", bad)
		}
	}
	if Validate(Content{Title: "A", StepTexts: []string{}}, Content{Title: "A", BodyMD: "Added rule", StepTexts: []string{}}) == nil {
		t.Fatal("added prose accepted")
	}
}
func TestTranslationProtectsRelativeLinksAndFragments(t *testing.T) {
	for _, body := range []string{"[Consent](../consent.md)", "[Consent](#consent)", "[Consent][ref]\n\n[ref]: ../consent.md", "[Consent](<../consent document.md>)"} {
		source := Content{Title: "Consent", BodyMD: body, StepTexts: []string{}}
		target := Content{Title: "Súhlas", BodyMD: strings.ReplaceAll(body, "Consent", "Súhlas"), StepTexts: []string{}}
		if err := Validate(source, target); err != nil {
			t.Fatal(err)
		}
		target.BodyMD = strings.ReplaceAll(target.BodyMD, "consent", "other")
		if Validate(source, target) == nil {
			t.Fatal("changed relative destination accepted")
		}
	}
}
func TestReadingBodyAndStrictJSON(t *testing.T) {
	for _, test := range []struct{ body, want string }{
		{"<!-- business-id: VOZ-CON-A -->\n<!-- relates-to: VOZ-CON-B -->\n# Title\n\nBody", "Body"}, {"# Title", ""}, {"Body", "Body"}, {"<!-- incomplete", "<!-- incomplete"}, {"# Other\nText", "# Other\nText"},
	} {
		if got := ReadingBody("Title", test.body); got != test.want {
			t.Fatalf("body=%q want=%q", got, test.want)
		}
	}
	for _, raw := range []string{`bad`, `{"title":"A","unknown":true}`, `{"title":"A"} {}`, `{"title":"A"} bad`} {
		if _, err := Decode([]byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	if c, err := Decode([]byte(`{"title":"A","bodyMd":"","stepTexts":[]}`)); err != nil || c.Title != "A" {
		t.Fatal(c, err)
	}
}

type testStore struct {
	job                            *Job
	claimErr, completeErr, failErr error
	completed, failed              bool
	cancel                         context.CancelFunc
}

func (s *testStore) ClaimLocalization(context.Context) (*Job, error) {
	if s.cancel != nil {
		s.cancel()
	}
	job := s.job
	s.job = nil
	return job, s.claimErr
}
func (s *testStore) CompleteLocalization(_ context.Context, _ Job, content Content, policy string) error {
	s.completed = true
	if policy != PolicyVersion || content.Title != "Cena za 100 gramov" {
		panic("wrong completion")
	}
	return s.completeErr
}
func (s *testStore) FailLocalization(context.Context, Job) error { s.failed = true; return s.failErr }

type testTranslator struct {
	content Content
	err     error
}

func (t testTranslator) Translate(context.Context, Content) (Content, error) { return t.content, t.err }
func TestDurableWorkerOutcomesAndCancellation(t *testing.T) {
	for _, test := range []struct {
		store                                   testStore
		translator                              testTranslator
		processed, failed, completed, wantError bool
	}{
		{store: testStore{}, processed: false},
		{store: testStore{claimErr: errors.New("db")}, wantError: true},
		{store: testStore{job: &Job{Source: sourceContent()}}, translator: testTranslator{err: errors.New("provider")}, processed: true, failed: true, wantError: true},
		{store: testStore{job: &Job{Source: sourceContent()}, failErr: errors.New("db")}, processed: true, failed: true, wantError: true},
		{store: testStore{job: &Job{Source: sourceContent()}, completeErr: errors.New("db")}, translator: testTranslator{content: slovakContent()}, processed: true, completed: true, wantError: true},
		{store: testStore{job: &Job{Source: sourceContent()}}, translator: testTranslator{content: slovakContent()}, processed: true, completed: true},
	} {
		processed, err := Step(context.Background(), &test.store, test.translator)
		if processed != test.processed || (err != nil) != test.wantError || test.store.failed != test.failed || test.store.completed != test.completed {
			t.Fatalf("outcome=%+v error=%v", test, err)
		}
	}
	for _, claimError := range []error{nil, errors.New("offline")} {
		ctx, cancel := context.WithCancel(context.Background())
		store := &testStore{claimErr: claimError, cancel: cancel}
		Run(ctx, store, testTranslator{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}
	ctx, cancel := context.WithCancel(context.Background())
	store := &testStore{job: &Job{Source: sourceContent()}, cancel: cancel}
	Run(ctx, store, testTranslator{content: slovakContent()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
}
func TestProviderUsesStructuredOutputAndDoesNotExposeErrorsOrCredentials(t *testing.T) {
	cases := []struct {
		body   string
		status int
		ok     bool
	}{
		{`{"status":"completed","output":[{"content":[{"type":"other"},{"type":"output_text","text":"{\"title\":\"Cena za 100 gramov\",\"bodyMd\":\"\",\"stepTexts\":[]}"}]}]}`, 200, true},
		{`secret provider error`, 429, false}, {`bad`, 200, false}, {`{"status":"incomplete"}`, 200, false}, {`{"status":"completed","output":[{"content":[{"type":"refusal"}]}]}`, 200, false},
		{`{"status":"completed","output":[{"content":[{"type":"output_text","text":"bad"}]}]}`, 200, false},
	}
	for _, tc := range cases {
		t.Run(tc.body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-key" || r.Method != "POST" {
					t.Error("wrong authentication")
				}
				request, _ := io.ReadAll(r.Body)
				for _, required := range []string{`"store":false`, `"json_schema"`, `"strict":true`, `"model":"test-model"`} {
					if !strings.Contains(string(request), required) {
						t.Errorf("missing %s", required)
					}
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			p := NewProvider("test-key", "test-model")
			p.endpoint = server.URL
			_, err := p.Translate(context.Background(), sourceContent())
			if (err == nil) != tc.ok {
				t.Fatal(err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("leaked provider body")
			}
		})
	}
	p := NewProvider("test-key", "test-model")
	if p.client.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("redirect allowed")
	}
	p.endpoint = "://invalid"
	if _, err := p.Translate(context.Background(), sourceContent()); err == nil {
		t.Fatal("invalid endpoint accepted")
	}
	p.endpoint = "https://example.test"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Translate(ctx, sourceContent()); err == nil {
		t.Fatal("cancel ignored")
	}
}
