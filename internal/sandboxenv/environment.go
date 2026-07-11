// Package sandboxenv prepares the environment used to start a process-sandbox
// wrapper. Dynamic-loader controls must not reach the wrapper itself: they are
// interpreted before sandbox-exec or Bubblewrap can establish confinement.
package sandboxenv

import "strings"

// Sanitize removes environment entries that can make a platform dynamic
// loader load code or modules before the sandbox wrapper starts.
func Sanitize(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if ok && unsafeLoaderVariable(name) {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func unsafeLoaderVariable(name string) bool {
	if strings.HasPrefix(name, "LD_") || strings.HasPrefix(name, "DYLD_") {
		return true
	}
	switch name {
	case "GCONV_PATH", "GETCONF_DIR", "GLIBC_TUNABLES", "LOCPATH", "MALLOC_TRACE", "NLSPATH":
		return true
	default:
		return false
	}
}
