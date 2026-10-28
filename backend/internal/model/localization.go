package model

// LocalizedContent is a presentation of one immutable English business revision.
// It never changes that revision's identity, source bytes, or approval status.
type LocalizedContent struct {
	RevisionID string   `json:"revisionId"`
	SourceHash string   `json:"sourceHash"`
	Version    int      `json:"version"`
	Title      string   `json:"title"`
	BodyMD     string   `json:"bodyMd"`
	StepTexts  []string `json:"stepTexts"`
}

type Localizations map[string]LocalizedContent
