package session

import "testing"

func TestEncodePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/Users/name/dev/project", "-Users-name-dev-project"},
		{"/home/user/repos/myrepo", "-home-user-repos-myrepo"},
		{"/Users/name/dev/repos/myproject", "-Users-name-dev-repos-myproject"},
	}
	for _, tt := range tests {
		got := EncodePath(tt.input)
		if got != tt.expected {
			t.Errorf("EncodePath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDecodePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"-Users-name-dev-project", "/Users/name/dev/project"},
		{"-home-user-repos-myrepo", "/home/user/repos/myrepo"},
	}
	for _, tt := range tests {
		got := DecodePath(tt.input)
		if got != tt.expected {
			t.Errorf("DecodePath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	// Round-trip works when directory names don't contain hyphens
	paths := []string{
		"/Users/name/dev/project",
		"/home/user/repos/myrepo",
	}
	for _, p := range paths {
		encoded := EncodePath(p)
		decoded := DecodePath(encoded)
		if decoded != p {
			t.Errorf("round-trip failed: %q → %q → %q", p, encoded, decoded)
		}
	}
}
