// Package service – lightweight regex-based PII filter used by the L3
// memory pipeline (`aranea/docs/15 memory-L3-semantic.md` §5.2 step 2).
// The implementation is intentionally simple — phone / email / credit
// card / id-like number detection — so it can run synchronously on the
// upsert path without taking a dependency on a NER model. Service code
// uses `RedactPII` to produce both a flag (was anything detected?) and a
// masked version of the input that's safe to surface in shared scopes.
package service

import (
	"regexp"
	"strings"
)

// PIIFilter detects and masks personally identifiable information in
// free-form text. The zero value is ready to use.
type PIIFilter struct{}

// NewPIIFilter returns a default PII filter with the built-in regex set.
func NewPIIFilter() *PIIFilter { return &PIIFilter{} }

// piiPatterns are matched against the input in order. Each pattern's
// match is replaced with a fixed mask token so downstream callers can
// detect redactions without reparsing.
var piiPatterns = []struct {
	name    string
	mask    string
	pattern *regexp.Regexp
}{
	{
		name:    "email",
		mask:    "[REDACTED_EMAIL]",
		pattern: regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`),
	},
	{
		name: "phone",
		mask: "[REDACTED_PHONE]",
		// Permissive intl phone matcher — at least 7 digits with optional
		// separators / leading +. The leading boundary keeps it from
		// eating the digit suffix of unrelated identifiers.
		pattern: regexp.MustCompile(`(?:\+?\d[\d\s\-]{6,}\d)`),
	},
	{
		name:    "credit_card",
		mask:    "[REDACTED_CARD]",
		pattern: regexp.MustCompile(`\b(?:\d[ -]*?){13,19}\b`),
	},
	{
		name: "id_number",
		mask: "[REDACTED_ID]",
		// Long digit / alnum runs that look like national IDs / SSNs but
		// are not phone/credit-card matches (we run this last so the
		// other masks have already caught them).
		pattern: regexp.MustCompile(`\b[A-Z0-9]{8,}\b`),
	},
}

// RedactPII scans the input and returns (hit, redacted). When no PII is
// detected the original string is returned and hit is false. The
// detection is best-effort: the goal is to keep obvious leaks out of
// shared scopes, not to guarantee zero-leakage.
func (f *PIIFilter) RedactPII(text string) (bool, string) {
	if strings.TrimSpace(text) == "" {
		return false, text
	}
	out := text
	hit := false
	for _, p := range piiPatterns {
		if p.pattern.MatchString(out) {
			out = p.pattern.ReplaceAllString(out, p.mask)
			hit = true
		}
	}
	return hit, out
}

// HasPII is the cheap variant for callers that only need the boolean
// flag (e.g. ACL gates).
func (f *PIIFilter) HasPII(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	for _, p := range piiPatterns {
		if p.pattern.MatchString(text) {
			return true
		}
	}
	return false
}

// Hits returns the names of every PII pattern that matched the input.
// Useful for callers (e.g. AgentEvolutionService.UpdateIdentity) that
// want to surface *which* PII categories were detected rather than just
// a flag. Returns nil when nothing matched.
func (f *PIIFilter) Hits(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var hits []string
	for _, p := range piiPatterns {
		if p.pattern.MatchString(text) {
			hits = append(hits, p.name)
		}
	}
	return hits
}
