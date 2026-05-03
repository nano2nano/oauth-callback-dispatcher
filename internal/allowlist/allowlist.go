package allowlist

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
)

var (
	ErrEmptyPattern      = errors.New("ALLOWED_ORIGIN_PATTERN is required")
	ErrUnanchoredPattern = errors.New("ALLOWED_ORIGIN_PATTERN must start with ^ and end with $")
	ErrUnescapedDot      = errors.New("ALLOWED_ORIGIN_PATTERN contains an unescaped literal dot")
	ErrInvalidOrigin     = errors.New("origin must be scheme://host[:port] without userinfo, path, query, or fragment")
)

type Allowlist struct {
	pattern string
	re      *regexp.Regexp
}

func New(pattern string, logger *slog.Logger) (*Allowlist, error) {
	if pattern == "" {
		return nil, ErrEmptyPattern
	}
	if !strings.HasPrefix(pattern, "^") || !strings.HasSuffix(pattern, "$") {
		return nil, ErrUnanchoredPattern
	}
	if hasUnescapedLiteralDot(pattern) {
		return nil, ErrUnescapedDot
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("compile ALLOWED_ORIGIN_PATTERN: %w", err)
	}
	a := &Allowlist{pattern: pattern, re: re}
	for _, candidate := range bypassCandidates {
		if a.Allows(candidate) {
			return nil, fmt.Errorf("ALLOWED_ORIGIN_PATTERN accepts bypass candidate %q", candidate)
		}
	}
	if strings.Contains(pattern, "https?://") && logger != nil {
		logger.Warn("ALLOWED_ORIGIN_PATTERN allows plain HTTP")
	}
	return a, nil
}

func (a *Allowlist) Allows(origin string) bool {
	normalized, err := NormalizeOrigin(origin)
	return err == nil && a != nil && a.re.MatchString(normalized)
}

func (a *Allowlist) Pattern() string {
	if a == nil {
		return ""
	}
	return a.pattern
}

// bypassCandidates are origins that any sane allowlist must reject.
// Each entry maps to a documented redirect_uri allowlist bypass class:
// foreign domain, plain HTTP variant, userinfo authority confusion
// (Pocket ID CVE-2026-28512), suffix concatenation and sub-suffix
// takeover (Authentik CVE-2024-52289 family), and the unescaped-dot
// regex mistake also exercised by hasUnescapedLiteralDot.
var bypassCandidates = []string{
	"https://evil.com",
	"http://evil.com",
	"https://app.myapp.localhost@evil.com",
	"https://attacker.com.myapp.localhost.evil.com",
	"https://myapp.localhost.evil.com",
	"https://appxmyappxlocalhost",
	"https://app.myapp.localhost.evil.com",
}

func NormalizeOrigin(origin string) (string, error) {
	u, err := url.Parse(origin)
	if err != nil {
		return "", ErrInvalidOrigin
	}
	if u.Scheme == "" || u.Host == "" || u.User != nil || u.Opaque != "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", ErrInvalidOrigin
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", ErrInvalidOrigin
	}
	return u.String(), nil
}

func hasUnescapedLiteralDot(pattern string) bool {
	escaped := false
	inClass := false
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		if escaped {
			escaped = false
			continue
		}
		switch ch {
		case '\\':
			escaped = true
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '.':
			if !inClass {
				return true
			}
		}
	}
	return false
}
