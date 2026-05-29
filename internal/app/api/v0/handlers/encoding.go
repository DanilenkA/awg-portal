package handlers

import (
	"encoding/base64"
	"strings"
)

// Base64UrlDecode decodes a base64 url encoded string.
// In comparison to the standard base64 encoding, the url encoding uses - instead of + and _ instead of /
// as well as . instead of =.
// If the input is not valid base64, it is returned unchanged (to support plain ASCII identifiers).
func Base64UrlDecode(in string) string {
	// If the string doesn't contain any base64-encoded markers, it's already a plain text identifier.
	if !strings.ContainsAny(in, "-_./+= ") {
		return in
	}

	in = strings.ReplaceAll(in, "-", "=")
	in = strings.ReplaceAll(in, "_", "/")
	in = strings.ReplaceAll(in, ".", "+")

	output, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		return in // fallback: return the adjusted string (better than garbage)
	}
	return string(output)
}
