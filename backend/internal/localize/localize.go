package localize

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"
)

const PolicyVersion = "sk-v2"

type Content struct {
	Title     string   `json:"title"`
	BodyMD    string   `json:"bodyMd"`
	StepTexts []string `json:"stepTexts"`
}
type Job struct {
	RevisionID string
	SourceHash string
	Lease      string
	Source     Content
}
type Store interface {
	ClaimLocalization(context.Context) (*Job, error)
	CompleteLocalization(context.Context, Job, Content, string) error
	FailLocalization(context.Context, Job) error
}
type Translator interface {
	Translate(context.Context, Content) (Content, error)
}

// Run drains durable jobs serially. Leases survive restarts; a failed revision
// retries at most three times and cannot stop English imports or page reads.
func Run(ctx context.Context, store Store, translator Translator, logger *slog.Logger) {
	for {
		processed, err := Step(ctx, store, translator)
		if err != nil {
			logger.Warn("specification localization deferred", "error", err)
		}
		if processed && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}
func Step(ctx context.Context, store Store, translator Translator) (bool, error) {
	job, err := store.ClaimLocalization(ctx)
	if err != nil || job == nil {
		return false, err
	}
	content, err := translator.Translate(ctx, job.Source)
	if err == nil {
		err = Validate(job.Source, content)
	}
	if err != nil {
		return true, errors.Join(err, store.FailLocalization(ctx, *job))
	}
	return true, store.CompleteLocalization(ctx, *job, content, PolicyVersion)
}

var protectedToken = regexp.MustCompile("https?://[^\\s<>]+|VOZ-[A-Z0-9-]+|`[^`]*`|[0-9]+(?:[.,][0-9]+)?|\\{[A-Za-z_][A-Za-z0-9_]*\\}")

var markdownLinkStart = regexp.MustCompile(`\]\(\s*`)
var markdownReferenceStart = regexp.MustCompile(`(?m)^[ \t]{0,3}\[[^\]\n]+\]:[ \t]*`)

// Read balanced/escaped destinations rather than stopping at a URL's first ')'.
func markdownDestination(text string, start int, inline bool) string {
	if start == len(text) {
		return ""
	}
	depth := 0
	angled := text[start] == '<'
	for i := start; i < len(text); i++ {
		character := text[i]
		if character == '\\' {
			i++
			continue
		}
		if angled {
			if character == '>' {
				return text[start : i+1]
			}
			continue
		}
		if strings.ContainsRune(" \t\r\n", rune(character)) {
			return text[start:i]
		}
		if inline {
			if character == '(' {
				depth++
			}
			if character == ')' {
				if depth == 0 {
					return text[start:i]
				}
				depth--
			}
		}
	}
	return text[start:]
}
func protected(text string) []string {
	values := protectedToken.FindAllString(text, -1)
	for _, pattern := range []struct {
		expression *regexp.Regexp
		inline     bool
	}{{markdownLinkStart, true}, {markdownReferenceStart, false}} {
		for _, match := range pattern.expression.FindAllStringIndex(text, -1) {
			if destination := markdownDestination(text, match[1], pattern.inline); destination != "" {
				values = append(values, destination)
			}
		}
	}
	slices.Sort(values)
	return values
}
func Validate(source, target Content) error {
	if strings.TrimSpace(target.Title) == "" || len(target.Title) > 1000 || len(target.BodyMD) > 200000 || target.StepTexts == nil || len(source.StepTexts) != len(target.StepTexts) {
		return errors.New("invalid localization structure")
	}
	before := append([]string{source.Title, source.BodyMD}, source.StepTexts...)
	after := append([]string{target.Title, target.BodyMD}, target.StepTexts...)
	for i, text := range before {
		if (strings.TrimSpace(text) == "") != (strings.TrimSpace(after[i]) == "") || !slices.Equal(protected(text), protected(after[i])) {
			return errors.New("localization changed a protected value or omitted content")
		}
	}
	return nil
}

// Reading prose omits source annotations; those remain untouched in Git/storage.
func ReadingBody(title, body string) string {
	body = strings.TrimSpace(body)
	for strings.HasPrefix(body, "<!--") {
		end := strings.Index(body, "-->")
		if end < 0 {
			break
		}
		body = strings.TrimSpace(body[end+3:])
	}
	lines := strings.SplitN(body, "\n", 2)
	if strings.TrimSpace(lines[0]) == "# "+title {
		if len(lines) == 1 {
			return ""
		}
		return strings.TrimSpace(lines[1])
	}
	return body
}

func Decode(raw []byte) (Content, error) {
	var content Content
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&content); err != nil {
		return content, err
	}
	var extra any
	if decoder.Decode(&extra) == io.EOF {
		return content, nil
	}
	return Content{}, errors.New("trailing localization data")
}
