package store

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Image visibility values used by uploads and image requests. "public" means
// the artwork was sourced from a public service; "private" means it was not.
const (
	VisibilityAll     = "all"
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

// VisibilityPolicy is the compiled form of the admin-managed regex lists plus
// the optional filters supplied by one image request.
//
// Exclude lists always hide a matching image. Include lists are allowlists:
// when an include list is non-empty, an image must match at least one entry in
// every non-empty include list (global and request-level lists are layered).
// Matching is case-insensitive and unanchored, so "spam" matches "SpamBot" and
// "Ad_Spam_42".
type VisibilityPolicy struct {
	excludeAuthors []*regexp.Regexp
	excludeTitles  []*regexp.Regexp
	includeAuthors []*regexp.Regexp
	includeTitles  []*regexp.Regexp

	// Request-specific filters. They are empty for the global-only policy used
	// by vote validation and admin ranking.
	reqExcludeAuthors []*regexp.Regexp
	reqExcludeTitles  []*regexp.Regexp
	reqIncludeAuthors []*regexp.Regexp
	reqIncludeTitles  []*regexp.Regexp

	visibility string
	service    string
	uploadedBy map[int64]struct{}
}

// ImageRequest carries the per-request public/private, provenance, uploader and
// black/white-list filters accepted by the public image endpoints.
type ImageRequest struct {
	Visibility     string
	Service        string
	UploadedBy     []int64
	IncludeAuthors []string
	ExcludeAuthors []string
	IncludeTitles  []string
	ExcludeTitles  []string
}

// NormalizeVisibility validates a visibility value. Empty means "all".
func NormalizeVisibility(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", VisibilityAll:
		return VisibilityAll, nil
	case VisibilityPublic:
		return VisibilityPublic, nil
	case VisibilityPrivate:
		return VisibilityPrivate, nil
	default:
		return "", fmt.Errorf("visibility must be one of all, public, private")
	}
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

// CompileVisibility compiles the stored pattern lists into a global-only
// policy. It never returns nil; an invalid pattern yields ErrInvalidFilter.
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

// CompileImageRequest combines the global visibility rules with filters from
// one request. When the request omits visibility, the admin default applies.
func CompileImageRequest(cfg AppConfig, req ImageRequest) (*VisibilityPolicy, error) {
	p, err := CompileVisibility(cfg)
	if err != nil {
		return p, err
	}

	visibility := strings.TrimSpace(req.Visibility)
	if visibility == "" {
		visibility = cfg.DefaultImageVisibility
	}
	visibility, err = NormalizeVisibility(visibility)
	if err != nil {
		return p, ErrInvalidFilter
	}
	p.visibility = visibility

	if p.reqExcludeAuthors, err = compilePatternList(req.ExcludeAuthors); err != nil {
		return p, ErrInvalidFilter
	}
	if p.reqExcludeTitles, err = compilePatternList(req.ExcludeTitles); err != nil {
		return p, ErrInvalidFilter
	}
	if p.reqIncludeAuthors, err = compilePatternList(req.IncludeAuthors); err != nil {
		return p, ErrInvalidFilter
	}
	if p.reqIncludeTitles, err = compilePatternList(req.IncludeTitles); err != nil {
		return p, ErrInvalidFilter
	}

	p.service = strings.ToLower(strings.TrimSpace(req.Service))
	if len(req.UploadedBy) > 0 {
		p.uploadedBy = make(map[int64]struct{}, len(req.UploadedBy))
		for _, id := range req.UploadedBy {
			if id > 0 {
				p.uploadedBy[id] = struct{}{}
			}
		}
		if len(p.uploadedBy) == 0 {
			p.uploadedBy = nil
		}
	}
	return p, nil
}

// Empty reports whether no global or request filter is configured.
func (p *VisibilityPolicy) Empty() bool {
	if p == nil {
		return true
	}
	return len(p.excludeAuthors) == 0 && len(p.excludeTitles) == 0 &&
		len(p.includeAuthors) == 0 && len(p.includeTitles) == 0 &&
		len(p.reqExcludeAuthors) == 0 && len(p.reqExcludeTitles) == 0 &&
		len(p.reqIncludeAuthors) == 0 && len(p.reqIncludeTitles) == 0 &&
		(p.visibility == "" || p.visibility == VisibilityAll) &&
		p.service == "" && len(p.uploadedBy) == 0
}

// Hidden reports whether an image is excluded by the configured rules.
func (p *VisibilityPolicy) Hidden(img *Image) bool {
	if img == nil {
		return false
	}
	if p == nil {
		return false
	}
	switch p.visibility {
	case VisibilityPublic:
		if !img.FromPublicService {
			return true
		}
	case VisibilityPrivate:
		if img.FromPublicService {
			return true
		}
	}
	if p.service != "" && !strings.Contains(strings.ToLower(img.ServiceName), p.service) {
		return true
	}
	if len(p.uploadedBy) > 0 {
		if img.UploadedBy == nil {
			return true
		}
		if _, ok := p.uploadedBy[*img.UploadedBy]; !ok {
			return true
		}
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
	if len(p.reqIncludeAuthors) > 0 && !matchesAny(p.reqIncludeAuthors, author) {
		return true
	}
	if len(p.includeTitles) > 0 && !matchesAny(p.includeTitles, title) {
		return true
	}
	if len(p.reqIncludeTitles) > 0 && !matchesAny(p.reqIncludeTitles, title) {
		return true
	}
	if matchesAny(p.excludeAuthors, author) || matchesAny(p.reqExcludeAuthors, author) {
		return true
	}
	if matchesAny(p.excludeTitles, title) || matchesAny(p.reqExcludeTitles, title) {
		return true
	}
	return false
}

// SQLFilter returns the subset of request filters that can be pushed into
// SQLite. Regex lists still need in-memory evaluation. Alias may be empty.
func (p *VisibilityPolicy) SQLFilter(alias string) (string, []any) {
	if p == nil {
		return "", nil
	}
	prefix := ""
	if alias = strings.TrimSpace(alias); alias != "" {
		prefix = alias + "."
	}
	where := ""
	args := make([]any, 0, 2)
	switch p.visibility {
	case VisibilityPublic:
		where += ` AND ` + prefix + `from_public_service = 1`
	case VisibilityPrivate:
		where += ` AND ` + prefix + `from_public_service = 0`
	}
	if p.service != "" {
		where += ` AND LOWER(` + prefix + `service_name) LIKE ?`
		args = append(args, "%"+p.service+"%")
	}
	if len(p.uploadedBy) > 0 {
		ids := make([]int64, 0, len(p.uploadedBy))
		for id := range p.uploadedBy {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		where += ` AND ` + prefix + `uploaded_by IN (` + placeholders(len(ids)) + `)`
		for _, id := range ids {
			args = append(args, id)
		}
	}
	return where, args
}

func matchesAny(patterns []*regexp.Regexp, value string) bool {
	for _, re := range patterns {
		if re.MatchString(value) {
			return true
		}
	}
	return false
}
