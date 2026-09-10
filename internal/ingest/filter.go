package ingest

import "strings"

// Filter decides which parsed PageViews the daemon persists. IncludeHosts,
// when non-empty, is an allowlist — only listed hosts pass. ExcludeHosts
// always wins over IncludeHosts, so a host can't be simultaneously included
// and excluded by conflicting flags. Hosts are matched normalized
// (lowercase); paths by prefix, mirroring isStaticAsset's existing
// prefix-check style rather than a full glob/regex engine.
type Filter struct {
	IncludeHosts []string
	ExcludeHosts []string
	ExcludePaths []string
}

// NewFilter builds a Filter from operator-supplied flag values, normalizing
// hosts the same way parseNginxLog normalizes a PageView's Host so
// comparisons in allowPageView are a plain string match.
func NewFilter(includeHosts, excludeHosts, excludePaths []string) Filter {
	return Filter{
		IncludeHosts: normalizeHosts(includeHosts),
		ExcludeHosts: normalizeHosts(excludeHosts),
		ExcludePaths: excludePaths,
	}
}

func normalizeHosts(hosts []string) []string {
	normalized := make([]string, len(hosts))
	for i, h := range hosts {
		normalized[i] = NormalizeHost(h)
	}
	return normalized
}

// allowPageView reports whether pv should be persisted under f. Pure — no
// I/O, safe to call per line on the hot ingest path. pv is taken by pointer
// (read-only; allowPageView never mutates it) purely to avoid copying
// PageView's ~128 bytes on every line.
func allowPageView(f Filter, pv *PageView) bool {
	if contains(f.ExcludeHosts, pv.Host) {
		return false
	}
	if len(f.IncludeHosts) > 0 && !contains(f.IncludeHosts, pv.Host) {
		return false
	}
	for _, p := range f.ExcludePaths {
		if strings.HasPrefix(pv.Path, p) {
			return false
		}
	}
	return true
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
