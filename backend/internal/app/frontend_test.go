package app

import "testing"

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
}
