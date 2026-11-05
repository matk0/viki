package specsync

import "regexp"

var repositoryPattern = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var workflowPattern = regexp.MustCompile(`^\.github/workflows/[A-Za-z0-9_-][A-Za-z0-9_.-]*\.ya?ml$`)
var artifactPrefixPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// ValidRepository accepts a canonical GitHub repository URL, never an arbitrary
// host or URL suffix supplied by an artifact. The workspace owns this identity.
func ValidRepository(repository string) bool { return repositoryPattern.MatchString(repository) }
