package store

import (
	"regexp"
	"strings"
)

// VisibilityPolicy is the compiled form of the admin-managed regex lists.
//
// Exclude lists always hide a matching image. Include lists are allowlists:
// when an include list is non-empty, an image must match at least one entry in
// every non-empty include list. Matching is case-insensitive and unanchored,
// so "spam" matches "SpamBot" and "Ad_Spam_42".
type VisibilityPolicy struct {
	excludeAuthors []*regexp.Regexp
	excludeTitles  []*regexp.Regexp
	includeAuthors []*regexp.Regexp
	includeTitles  []*regexp.Regexp
}

// CompilePattern compiles one user-provided regex as a case-insensitive
// pattern. An existing (?i) prefix is kept as-is.
func CompilePattern(pattern string) (*regexp.Regexp, error) {
	pattern = trimPattern(pattern)
	if !strings.HasPrefix(pattern, "(?i)") {
		pattern = "(?i)" + pattern
	}
	return regexp.Compile(pattern)
}

func compilePatternList(patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := CompilePattern(pattern)
		if err != nil {
			return nil, err
		}
		out = append(out, re)
	}
	return out, nil
}

// CompileVisibility compiles the stored pattern lists into a policy. It
// never returns nil; an invalid pattern yields ErrInvalidFilter.
func CompileVisibility(cfg AppConfig) (*VisibilityPolicy, error) {
	p := &VisibilityPolicy{}
	var err error
	if p.excludeAuthors, err = compilePatternList(cfg.ExcludeAuthors); err != nil {
		return p, ErrInvalidFilter
	}
	if p.excludeTitles, err = compilePatternList(cfg.ExcludeTitles); err != nil {
		return p, ErrInvalidFilter
	}
	if p.includeAuthors, err = compilePatternList(cfg.IncludeAuthors); err != nil {
		return p, ErrInvalidFilter
	}
	if p.includeTitles, err = compilePatternList(cfg.IncludeTitles); err != nil {
		return p, ErrInvalidFilter
	}
	return p, nil
}

// Empty reports whether no pattern list is configured.
func (p *VisibilityPolicy) Empty() bool {
	if p == nil {
		return true
	}
	return len(p.excludeAuthors) == 0 && len(p.excludeTitles) == 0 &&
		len(p.includeAuthors) == 0 && len(p.includeTitles) == 0
}

// Hidden reports whether an image is excluded by the configured lists.
func (p *VisibilityPolicy) Hidden(img *Image) bool {
	if img == nil {
		return false
	}
	return p.HiddenMetadata(img.Author, img.Title)
}

// HiddenMetadata applies the allow/deny lists to an author/title pair.
func (p *VisibilityPolicy) HiddenMetadata(author, title string) bool {
	if p == nil || p.Empty() {
		return false
	}
	if len(p.includeAuthors) > 0 && !matchesAny(p.includeAuthors, author) {
		return true
	}
	if len(p.includeTitles) > 0 && !matchesAny(p.includeTitles, title) {
		return true
	}
	if matchesAny(p.excludeAuthors, author) {
		return true
	}
	if matchesAny(p.excludeTitles, title) {
		return true
	}
	return false
}

func matchesAny(patterns []*regexp.Regexp, value string) bool {
	for _, re := range patterns {
		if re.MatchString(value) {
			return true
		}
	}
	return false
}
