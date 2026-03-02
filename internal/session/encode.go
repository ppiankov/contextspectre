package session

import (
	"strings"
)

// EncodePath converts a filesystem path to Claude Code's project directory name.
// /Users/name/dev/project → -Users-name-dev-project
func EncodePath(fsPath string) string {
	// Replace all "/" with "-", then prepend "-"
	encoded := strings.ReplaceAll(fsPath, "/", "-")
	if !strings.HasPrefix(encoded, "-") {
		encoded = "-" + encoded
	}
	return encoded
}

// DecodePath converts a Claude Code project directory name back to a filesystem path.
// -Users-name-dev-project → /Users/name/dev/project
// Note: this is ambiguous when directory names contain literal hyphens.
// Use ValidateDecodedPath to check if the result exists on disk.
func DecodePath(dirName string) string {
	// Remove leading "-" and replace remaining "-" with "/"
	decoded := dirName
	if strings.HasPrefix(decoded, "-") {
		decoded = decoded[1:]
	}
	decoded = strings.ReplaceAll(decoded, "-", "/")
	if !strings.HasPrefix(decoded, "/") {
		decoded = "/" + decoded
	}
	return decoded
}
