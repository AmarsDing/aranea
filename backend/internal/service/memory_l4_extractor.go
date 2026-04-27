// Package service – L4 第二阶段实体抽取启发式。
//
// 第二阶段规范（§12 Phase 2）要求轻量抽取器：扫描事实与 episode 中的常见技术/框架/公司
// 名，转为知识图谱实体，并反向链回源事实/episode。
//
// 本文件提供基于小词典的匹配器，为每次命中产出规范名与类型。刻意
// 零依赖 —— 无 LLM、无嵌入 —— 以便在请求路径内联运行。第三阶段及以后可将 `scanExtractionMatches` 换为
// 模型驱动抽取器，调用点不变。
package service

import (
	"strings"
	"unicode"

	"arenea/backend/internal/domain"
)

// extractionTerm 为种子词典的一行。别名在源文本中按词边界、不区分大小写匹配。
type extractionTerm struct {
	Name    string
	Type    domain.EntityType
	Aliases []string
}

// extractionDictionary 为人工筛选的种子表。名称刻意短而高精，避免模糊匹配污染图谱。
// 仅当该词不太可能出现在无关正文时再添加（如 "Go" 有风险但仍可接受，因
// 匹配器要求整词边界）。
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

// extractionMatch 为扫描器产出的一则（规范名、实体类型）命中，别名列表仅保留
// 源文中实际出现的变体。
type extractionMatch struct {
	Name    string
	Type    domain.EntityType
	Aliases []string
}

// scanExtractionMatches 对 `text` 遍历词典，每个规范名至多一条。匹配顺序稳定，
// 重复调用报告一致。
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

// containsWord 判断已小写的 `haystack` 中 `needle` 是否被非字母数字包围。`haystack` 预期
// 首尾加空格，以便匹配原串开头/结尾的词。
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
		// 边界检查：任一侧为字母数字则不算，故 "reaction" 中的 "react" 不匹配。
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

// mergeAliases 合并两路别名切片并保持顺序（已有项在前）。同一规范名由
// 多行词典命中时使用。
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
