package analyzer

import "strings"

// ModelTurn records a model and its turn count for mixed-model sessions.
type ModelTurn struct {
	Model     string
	TurnCount int
}

// AbbreviateModel converts a full Claude model ID to a 5-char display abbreviation.
// Examples:
//
//	"claude-opus-4-6"    → "O4.6 "
//	"claude-sonnet-4-6"  → "S4.6 "
//	"claude-haiku-4-5-*" → "H4.5 "
//	"" or unknown        → "?    "
//
// The result is always padded to 5 characters.
func AbbreviateModel(model string) string {
	abbr := abbreviateModelInner(model)
	// Pad to fixed 5 chars
	for len(abbr) < 5 {
		abbr += " "
	}
	return abbr
}

func abbreviateModelInner(model string) string {
	if model == "" {
		return "?"
	}
	lo := strings.ToLower(model)

	var prefix string
	var rest string
	switch {
	case strings.Contains(lo, "opus"):
		prefix = "O"
		rest = lo[strings.Index(lo, "opus")+len("opus"):]
	case strings.Contains(lo, "sonnet"):
		prefix = "S"
		rest = lo[strings.Index(lo, "sonnet")+len("sonnet"):]
	case strings.Contains(lo, "haiku"):
		prefix = "H"
		rest = lo[strings.Index(lo, "haiku")+len("haiku"):]
	default:
		return "?"
	}

	// rest looks like "-4-6" or "-4-5-20251001" or "-4-8-20260301"
	// Extract major.minor version: skip leading "-"
	rest = strings.TrimLeft(rest, "-")
	// Split on "-" to get ["4", "6", ...] or ["4", "5", "20251001"]
	parts := strings.SplitN(rest, "-", 3)
	if len(parts) < 2 {
		return prefix + "?"
	}
	major := parts[0]
	minor := parts[1]
	// Truncate minor to avoid long dates sneaking through (e.g. "20251001" → not a minor)
	if len(minor) > 2 {
		minor = minor[:2]
	}
	return prefix + major + "." + minor
}
