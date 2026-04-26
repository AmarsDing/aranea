// Package service – L4 Phase 2 entity-extraction heuristic.
//
// The Phase 2 spec (§12 Phase 2) calls for a lightweight extractor that
// scans facts and episodes for well-known tech / framework / company
// names and turns them into knowledge-graph entities, with a reverse
// link back to the source fact / episode.
//
// This file ships a small dictionary-based matcher that produces the
// canonical entity name + type for each match. It is deliberately
// dependency-free — no LLMs, no embeddings — so it can run inline in the
// request path. Phase 3+ can replace `scanExtractionMatches` with a
// model-driven extractor without disturbing the call sites.
package service

import (
	"strings"
	"unicode"

	"arenea/backend/internal/domain"
)

// extractionTerm is one row of the seed dictionary. Aliases are matched
// case-insensitively against word-boundary tokens in the source text.
type extractionTerm struct {
	Name    string
	Type    domain.EntityType
	Aliases []string
}

// extractionDictionary is the curated seed list. Names are intentionally
// short and high-precision so the extractor avoids polluting the graph
// with ambiguous matches. Add entries here only if the term is unlikely
// to appear in unrelated prose (e.g. "Go" is risky but accepted because
// the matcher requires whole-word boundaries).
var extractionDictionary = []extractionTerm{
	{Name: "React", Type: domain.EntityFramework, Aliases: []string{"react", "reactjs", "react.js"}},
	{Name: "React 19", Type: domain.EntityFramework, Aliases: []string{"react 19", "react19"}},
	{Name: "Vue", Type: domain.EntityFramework, Aliases: []string{"vue", "vuejs", "vue.js"}},
	{Name: "Vue 3", Type: domain.EntityFramework, Aliases: []string{"vue 3", "vue3"}},
	{Name: "Svelte", Type: domain.EntityFramework, Aliases: []string{"svelte", "sveltekit"}},
	{Name: "Next.js", Type: domain.EntityFramework, Aliases: []string{"nextjs", "next.js"}},
	{Name: "Nuxt", Type: domain.EntityFramework, Aliases: []string{"nuxt", "nuxtjs", "nuxt.js"}},
	{Name: "Vite", Type: domain.EntityFramework, Aliases: []string{"vite"}},
	{Name: "Vitest", Type: domain.EntityFramework, Aliases: []string{"vitest"}},
	{Name: "Jest", Type: domain.EntityFramework, Aliases: []string{"jest"}},
	{Name: "Tailwind", Type: domain.EntityFramework, Aliases: []string{"tailwind", "tailwindcss"}},
	{Name: "Quasar", Type: domain.EntityFramework, Aliases: []string{"quasar"}},
	{Name: "FastAPI", Type: domain.EntityFramework, Aliases: []string{"fastapi"}},
	{Name: "Django", Type: domain.EntityFramework, Aliases: []string{"django"}},
	{Name: "Flask", Type: domain.EntityFramework, Aliases: []string{"flask"}},
	{Name: "Spring Boot", Type: domain.EntityFramework, Aliases: []string{"spring boot", "springboot"}},

	{Name: "Go", Type: domain.EntityTech, Aliases: []string{"golang"}},
	{Name: "TypeScript", Type: domain.EntityTech, Aliases: []string{"typescript"}},
	{Name: "JavaScript", Type: domain.EntityTech, Aliases: []string{"javascript"}},
	{Name: "Python", Type: domain.EntityTech, Aliases: []string{"python"}},
	{Name: "Rust", Type: domain.EntityTech, Aliases: []string{"rust"}},
	{Name: "Java", Type: domain.EntityTech, Aliases: []string{"java"}},
	{Name: "Kotlin", Type: domain.EntityTech, Aliases: []string{"kotlin"}},
	{Name: "Swift", Type: domain.EntityTech, Aliases: []string{"swift"}},
	{Name: "C#", Type: domain.EntityTech, Aliases: []string{"c#", "csharp"}},
	{Name: "C++", Type: domain.EntityTech, Aliases: []string{"c++", "cpp"}},

	{Name: "Postgres", Type: domain.EntityTech, Aliases: []string{"postgres", "postgresql"}},
	{Name: "MySQL", Type: domain.EntityTech, Aliases: []string{"mysql"}},
	{Name: "SQLite", Type: domain.EntityTech, Aliases: []string{"sqlite", "sqlite3"}},
	{Name: "Redis", Type: domain.EntityTech, Aliases: []string{"redis"}},
	{Name: "MongoDB", Type: domain.EntityTech, Aliases: []string{"mongodb", "mongo"}},
	{Name: "ClickHouse", Type: domain.EntityTech, Aliases: []string{"clickhouse"}},
	{Name: "Kafka", Type: domain.EntityTech, Aliases: []string{"kafka"}},
	{Name: "RabbitMQ", Type: domain.EntityTech, Aliases: []string{"rabbitmq"}},
	{Name: "Elasticsearch", Type: domain.EntityTech, Aliases: []string{"elasticsearch", "elastic search"}},

	{Name: "Docker", Type: domain.EntityTech, Aliases: []string{"docker"}},
	{Name: "Kubernetes", Type: domain.EntityTech, Aliases: []string{"kubernetes", "k8s"}},
	{Name: "AWS", Type: domain.EntityCompany, Aliases: []string{"aws", "amazon web services"}},
	{Name: "GCP", Type: domain.EntityCompany, Aliases: []string{"gcp", "google cloud"}},
	{Name: "Azure", Type: domain.EntityCompany, Aliases: []string{"azure"}},

	{Name: "OpenAI", Type: domain.EntityCompany, Aliases: []string{"openai"}},
	{Name: "Anthropic", Type: domain.EntityCompany, Aliases: []string{"anthropic"}},
	{Name: "Google", Type: domain.EntityCompany, Aliases: []string{"google"}},
	{Name: "Microsoft", Type: domain.EntityCompany, Aliases: []string{"microsoft"}},

	{Name: "GPT-4", Type: domain.EntityTech, Aliases: []string{"gpt-4", "gpt4"}},
	{Name: "Claude", Type: domain.EntityTech, Aliases: []string{"claude"}},
	{Name: "Gemini", Type: domain.EntityTech, Aliases: []string{"gemini"}},
}

// extractionMatch is one (canonical name, entity type) hit produced by
// the scanner, with the alias list trimmed to whatever variants were
// actually observed in the source text.
type extractionMatch struct {
	Name    string
	Type    domain.EntityType
	Aliases []string
}

// scanExtractionMatches walks the dictionary against `text` and returns
// one entry per canonical name observed. The match is order-stable so
// repeat calls produce identical reports.
func scanExtractionMatches(text string) []extractionMatch {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	lower := " " + strings.ToLower(text) + " "
	seen := map[string]int{}
	var out []extractionMatch
	for _, term := range extractionDictionary {
		var observed []string
		for _, alias := range term.Aliases {
			if alias == "" {
				continue
			}
			if !containsWord(lower, alias) {
				continue
			}
			observed = append(observed, alias)
		}
		if len(observed) == 0 {
			continue
		}
		key := string(term.Type) + "|" + strings.ToLower(term.Name)
		if idx, ok := seen[key]; ok {
			out[idx].Aliases = mergeAliases(out[idx].Aliases, observed)
			continue
		}
		seen[key] = len(out)
		out = append(out, extractionMatch{
			Name:    term.Name,
			Type:    term.Type,
			Aliases: observed,
		})
	}
	return out
}

// containsWord checks whether `needle` appears in the already-lowercased
// `haystack` surrounded by non-alphanumeric characters. The haystack is
// expected to be wrapped with leading + trailing spaces so callers can
// still match terms at the start / end of the original string.
func containsWord(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(haystack[start:], needle)
		if idx < 0 {
			return false
		}
		pos := start + idx
		// Boundary check: alphanumerics on either side disqualify so
		// "react" inside "reaction" never matches.
		if pos > 0 {
			r := rune(haystack[pos-1])
			if isWordChar(r) {
				start = pos + 1
				continue
			}
		}
		end := pos + len(needle)
		if end < len(haystack) {
			r := rune(haystack[end])
			if isWordChar(r) {
				start = pos + 1
				continue
			}
		}
		return true
	}
}

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// mergeAliases returns the union of two alias slices preserving order
// (existing entries first). Used when the same canonical name was
// reached by multiple dictionary rows.
func mergeAliases(existing, more []string) []string {
	seen := map[string]bool{}
	for _, a := range existing {
		seen[a] = true
	}
	out := append([]string(nil), existing...)
	for _, a := range more {
		if seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	return out
}
