package domain

import (
	"log/slog"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// LogSanitizeChange applies sanitizeFn to raw, logs when the value changes, and writes
// the sanitized value to dest. Raw and sanitized values are intentionally omitted.
func LogSanitizeChange(
	providerType string,
	providerName string,
	field string,
	raw string,
	sanitizeFn func() string,
	dest *string,
) {
	sanitized := sanitizeFn()
	if sanitized != raw {
		message := "sanitization modified field value from external provider"
		if sanitized == "" {
			message = "sanitization cleared field value from external provider"
		}
		slog.Warn(message,
			"provider_type", SanitizeString(providerType, 64),
			"provider", SanitizeString(providerName, 128),
			"field", SanitizeString(field, 64),
		)
	}
	*dest = sanitized
}

var reservedUserIdentifiers = map[string]struct{}{
	"all":               {},
	"new":               {},
	"id":                {},
	CtxSystemAdminId:    {},
	CtxUnknownUserId:    {},
	CtxSystemLdapSyncer: {},
	CtxSystemWgImporter: {},
	CtxSystemV1Migrator: {},
	CtxSystemDBMigrator: {},
}

// SanitizeString normalizes to NFC, trims leading and trailing whitespace, strips Unicode
// control and format characters, drops invalid UTF-8 bytes, and truncates the result to
// maxLen runes. If maxLen <= 0, returns "".
//
// The control/Cf filter MUST run BEFORE the NFC normalisation, not
// after, otherwise it can break the canonical combining-sequence
// invariant. Concrete counter-example that the old order
// (NFC → filter → trim) mishandled:
//
//	input  = "A\t̀"   // A, TAB, combining grave accent
//	once   = "A" + U+0300   // TAB was filtered, A and combining
//	                         // left adjacent but in DECOMPOSED form
//	                         // (NFC did not merge them because TAB
//	                         // was sitting between them at NFC time)
//	twice  = "À"      // NFC now merges A+U+0300 → U+00C0
//	once != twice     //NOT idempotent
//
// The fix: filter first, normalise second. After the filter removes
// the TAB, the surviving runes are A and U+0300 with nothing
// between them, and the subsequent NFC pass correctly composes them
// into U+00C0 in a single pass. A second NFC pass after the filter
// is unnecessary because Unicode guarantees the result of a single
// pass is canonical.
func SanitizeString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}

	s = strings.TrimSpace(s)

	// 1. Strip invalid UTF-8 bytes and Unicode control / format
	//    characters. We do this on the RAW input (before NFC) so
	//    that a control character sitting between a base character
	//    and its combining mark does not break the subsequent
	//    canonical composition.
	var b strings.Builder
	b.Grow(len(s))
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		if r == utf8.RuneError && size == 1 {
			continue
		}
		if !unicode.IsControl(r) && !unicode.Is(unicode.Cf, r) {
			b.WriteRune(r)
		}
	}
	s = b.String()

	// 2. Canonical composition (NFC). Now that control chars are
	//    gone, base + combining sequences that the original input
	//    split up can be composed into precomposed forms.
	s = norm.NFC.String(s)

	if utf8.RuneCountInString(s) > maxLen {
		runes := []rune(s)
		s = string(runes[:maxLen])
	}

	return strings.TrimSpace(s)
}

// SanitizeEmail applies SanitizeString first, then returns "" if the original s
// contains CR/LF or if the sanitized result is not a plain email address.
func SanitizeEmail(s string, maxLen int) string {
	if strings.ContainsRune(s, '\r') || strings.ContainsRune(s, '\n') {
		return ""
	}

	sanitized := SanitizeString(s, maxLen)

	if sanitized == "" || strings.Count(sanitized, "@") != 1 {
		return ""
	}
	addr, err := mail.ParseAddress(sanitized)
	if err != nil || addr.Name != "" || addr.Address != sanitized {
		return ""
	}

	return sanitized
}

// SanitizePhone applies SanitizeString first, then removes all characters not in the
// set [0-9+\-() .]. Returns "" if the result after filtering is empty.
func SanitizePhone(s string, maxLen int) string {
	sanitized := SanitizeString(s, maxLen)

	// Remove all characters not in [0-9+\-() .]
	var b strings.Builder
	b.Grow(len(sanitized))
	for _, r := range sanitized {
		if isAllowedPhoneRune(r) {
			b.WriteRune(r)
		}
	}
	result := strings.TrimSpace(b.String())

	if result == "" {
		return ""
	}

	return result
}

// isAllowedPhoneRune reports whether r is in the allowed phone character set [0-9+\-() .].
func isAllowedPhoneRune(r rune) bool {
	switch {
	case r >= '0' && r <= '9':
		return true
	case r == '+', r == '-', r == '(', r == ')', r == ' ', r == '.':
		return true
	default:
		return false
	}
}

// SanitizeIdentifier applies SanitizeString first, then returns "" if the result equals
// a reserved user identifier (case-sensitive, exact match).
func SanitizeIdentifier(s string, maxLen int) string {
	sanitized := SanitizeString(s, maxLen)

	if IsReservedUserIdentifier(UserIdentifier(sanitized)) {
		return ""
	}

	return sanitized
}

func IsReservedUserIdentifier(identifier UserIdentifier) bool {
	_, reserved := reservedUserIdentifiers[string(identifier)]
	return reserved
}
