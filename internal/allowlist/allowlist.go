package allowlist

import (
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

var (
	ErrEmptyPattern      = errors.New("ALLOWED_ORIGIN_PATTERN is required")
	ErrUnanchoredPattern = errors.New("ALLOWED_ORIGIN_PATTERN must start with ^ and end with $")
	ErrUnescapedDot      = errors.New("ALLOWED_ORIGIN_PATTERN contains an unescaped literal dot")
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
	return a != nil && a.re.MatchString(origin)
}

func (a *Allowlist) Pattern() string {
	if a == nil {
		return ""
	}
	return a.pattern
}

var bypassCandidates = []string{
	"https://evil.com",
	"http://evil.com",
	"https://attacker.com.myapp.localhost.evil.com",
	"https://myapp.localhost.evil.com",
	"https://appxmyappxlocalhost",
	"https://app.myapp.localhost.evil.com",
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
