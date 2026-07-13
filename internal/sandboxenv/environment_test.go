package sandboxenv

import (
	"slices"
	"testing"
)

func TestSanitizeRemovesPreSandboxLoaderControls(t *testing.T) {
	got := Sanitize([]string{
		"PATH=/usr/bin",
		"LD_PRELOAD=./project-owned.so",
		"LD_LIBRARY_PATH=./project-libs",
		"DYLD_INSERT_LIBRARIES=./project-owned.dylib",
		"GCONV_PATH=./modules",
		"SAFE=value",
	})
	want := []string{"PATH=/usr/bin", "SAFE=value"}
	if !slices.Equal(got, want) {
		t.Fatalf("sanitized environment = %#v, want %#v", got, want)
	}
}
