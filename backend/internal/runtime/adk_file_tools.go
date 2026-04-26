package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"arenea/backend/internal/domain"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

const fileToolMaxReadBytes = 1024 * 1024

// Tool descriptions are written to be both informative and protective.
// Each one explicitly states (1) what the tool does, (2) what data class
// it operates on (filesystem bytes, datetime, or HTTP), and (3) the kinds
// of questions that MUST NOT trigger the tool. Without this hardening the
// model has historically called read_file / list_files to answer questions
// about teams, members, sessions, providers — none of which live on disk.
// Tool descriptions deliberately avoid curly-brace argument examples
// (e.g. "{ path }") because ADK's instruction processor treats `{name}`
// patterns as session-state placeholders and will fail injection if
// the descriptions reach a system prompt template.
const (
	readFileToolDescription = "Read raw bytes of a UTF-8 text file located inside the workspace source tree. " +
		"Argument: path (string) relative to the project root, e.g. \"backend/internal/domain/models.go\". " +
		"Use only when the user explicitly asks to read or inspect a source file. " +
		"DO NOT use to answer questions about teams, members, sessions, agents, providers, models, dialog mode or any in-app metadata — that information is supplied in the Runtime Context block, not on disk."

	listFilesToolDescription = "List files and directories under a workspace path. " +
		"Argument: path (string), empty or \".\" lists the project root. " +
		"Use only for source-tree exploration when the user asked about files or folders. " +
		"DO NOT use to count team members, sessions, agents or any in-app entity; those counts are present in the Runtime Context block."

	writeFileToolDescription = "Create or overwrite a UTF-8 file inside the workspace sandbox. " +
		"Arguments: path (string), content (string). Parent directories are created automatically. " +
		"Use only when the user explicitly asked to save, write or generate a file. " +
		"DO NOT use to take notes about the conversation, persist memory or store agent state."

	editFileToolDescription = "Replace exactly one text occurrence in an existing workspace file. " +
		"Arguments: path (string), old_string (string), new_string (string). Fails if old_string is missing or ambiguous. " +
		"Use only after read_file confirmed the exact target text. " +
		"DO NOT call speculatively — if you don't already know the precise old_string, refuse and ask the user."

	datetimeToolDescription = "Return the current local and UTC clock time as RFC3339 strings. " +
		"Use only when the user asks for the current time or the answer depends on now. " +
		"DO NOT call to ground unrelated reasoning; the Runtime Context already records when the session started."

	webFetchToolDescription = "Fetch a single public HTTP or HTTPS URL and return a truncated plain-text preview. " +
		"Argument: url (string). Use only when the user explicitly provided a URL or asked you to look something up online. " +
		"DO NOT use to query application data, internal databases or anything described in the Runtime Context. " +
		"If the same URL fails twice, stop and report the limitation instead of retrying."
)

type fileToolPathArgs struct {
	Path string `json:"path"`
}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Deliver bool   `json:"deliver,omitempty"`
}

type editFileArgs struct {
	Path      string `json:"path"`
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}

type webFetchArgs struct {
	URL string `json:"url"`
}

func adkFilesystemTools() ([]tool.Tool, error) {
	definitions := []struct {
		name        string
		description string
		factory     func() (tool.Tool, error)
	}{
		{
			name:        "read_file",
			description: readFileToolDescription,
			factory: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "read_file",
					Description: readFileToolDescription,
				}, runReadFileTool)
			},
		},
		{
			name:        "list_files",
			description: listFilesToolDescription,
			factory: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "list_files",
					Description: listFilesToolDescription,
				}, runListFilesTool)
			},
		},
		{
			name:        "write_file",
			description: writeFileToolDescription,
			factory: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "write_file",
					Description: writeFileToolDescription,
				}, runWriteFileTool)
			},
		},
		{
			name:        "edit_file",
			description: editFileToolDescription,
			factory: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "edit_file",
					Description: editFileToolDescription,
				}, runEditFileTool)
			},
		},
	}

	tools := make([]tool.Tool, 0, len(definitions))
	for _, definition := range definitions {
		t, err := definition.factory()
		if err != nil {
			return nil, fmt.Errorf("create ADK filesystem tool %s: %w", definition.name, err)
		}
		tools = append(tools, t)
	}
	return tools, nil
}

func adkRuntimeTools(req GenerateRequest) ([]tool.Tool, error) {
	tools, err := adkFilesystemTools()
	if err != nil {
		return nil, err
	}
	extras, err := adkUtilityTools()
	if err != nil {
		return nil, err
	}
	tools = append(tools, extras...)
	return filterADKToolsBySettings(tools, req.ToolSettings), nil
}

func adkUtilityTools() ([]tool.Tool, error) {
	definitions := []struct {
		name    string
		factory func() (tool.Tool, error)
	}{
		{
			name: "datetime",
			factory: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "datetime",
					Description: datetimeToolDescription,
				}, runDateTimeTool)
			},
		},
		{
			name: "web_fetch",
			factory: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "web_fetch",
					Description: webFetchToolDescription,
				}, runWebFetchTool)
			},
		},
	}
	tools := make([]tool.Tool, 0, len(definitions))
	for _, definition := range definitions {
		t, err := definition.factory()
		if err != nil {
			return nil, fmt.Errorf("create ADK utility tool %s: %w", definition.name, err)
		}
		tools = append(tools, t)
	}
	return tools, nil
}

func filterADKToolsBySettings(tools []tool.Tool, settings *domain.AgentRuntimeSettings) []tool.Tool {
	if settings == nil {
		return tools
	}
	if !settings.ToolsEnabled {
		return nil
	}
	profile := strings.TrimSpace(settings.ToolsProfile)
	allow := runtimeToolSet(jsonStringList(settings.ToolsAllowJSON))
	deny := runtimeToolSet(jsonStringList(settings.ToolsDenyJSON))
	out := make([]tool.Tool, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		name := t.Name()
		allowed := profile == "" || profile == "full" || runtimeProfileAllows(profile, name) || allow[name]
		if !allowed || deny[name] {
			continue
		}
		out = append(out, t)
	}
	return out
}

func jsonStringList(raw string) []string {
	var items []string
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &items) != nil {
		return nil
	}
	return items
}

func runtimeToolSet(items []string) map[string]bool {
	out := map[string]bool{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		switch item {
		case "group:filesystem":
			for _, key := range []string{"read_file", "write_file", "list_files", "edit_file"} {
				out[key] = true
			}
		case "group:web":
			out["web_fetch"] = true
		case "edit":
			out["edit_file"] = true
		case "browser":
			out["web_fetch"] = true
		case "":
		default:
			out[item] = true
		}
	}
	return out
}

// runtimeProfileAllows decides whether a tool name is allowed under a
// given profile. The profile vocabulary mirrors service.toolProfiles
// and accepts both canonical and legacy names so settings stored
// before the rename keep working without a database migration.
func runtimeProfileAllows(profile string, name string) bool {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "chat_only", "minimal":
		// chat_only intentionally exposes no tools — the agent must
		// answer purely from the prompt and the runtime context.
		return false
	case "read_only", "safe":
		return name == "datetime" || name == "read_file" || name == "list_files"
	case "coding":
		return name == "datetime" || name == "read_file" || name == "write_file" || name == "list_files" || name == "edit_file" || name == "web_fetch"
	case "research":
		return name == "datetime" || name == "read_file" || name == "list_files" || name == "web_fetch"
	case "system_admin", "full":
		return true
	default:
		return false
	}
}

func runReadFileTool(_ tool.Context, args fileToolPathArgs) (map[string]any, error) {
	path, rel, err := resolveWorkspacePath(args.Path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return runListFilesTool(nil, fileToolPathArgs{Path: rel})
	}
	if info.Size() > fileToolMaxReadBytes {
		return nil, fmt.Errorf("file %q is too large (%d bytes, max %d)", rel, info.Size(), fileToolMaxReadBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"path":    rel,
		"content": string(data),
		"size":    len(data),
	}, nil
}

func runListFilesTool(_ tool.Context, args fileToolPathArgs) (map[string]any, error) {
	path, rel, err := resolveWorkspacePath(args.Path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		info, statErr := entry.Info()
		item := map[string]any{
			"name":  entry.Name(),
			"isDir": entry.IsDir(),
		}
		if statErr == nil {
			item["size"] = info.Size()
			item["modTime"] = info.ModTime().Format("2006-01-02T15:04:05Z07:00")
		}
		items = append(items, item)
	}
	return map[string]any{
		"path":  rel,
		"items": items,
	}, nil
}

func runWriteFileTool(_ tool.Context, args writeFileArgs) (map[string]any, error) {
	path, rel, err := resolveWorkspacePath(args.Path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err = os.WriteFile(path, []byte(args.Content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{
		"path":    rel,
		"written": len(args.Content),
	}, nil
}

func runEditFileTool(_ tool.Context, args editFileArgs) (map[string]any, error) {
	if args.OldString == "" {
		return nil, fmt.Errorf("old_string is required")
	}
	path, rel, err := resolveWorkspacePath(args.Path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)
	count := strings.Count(content, args.OldString)
	if count == 0 {
		return nil, fmt.Errorf("old_string was not found in %q", rel)
	}
	if count > 1 {
		return nil, fmt.Errorf("old_string matched %d times in %q; provide more context", count, rel)
	}
	next := strings.Replace(content, args.OldString, args.NewString, 1)
	if err = os.WriteFile(path, []byte(next), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{
		"path":         rel,
		"replacements": 1,
	}, nil
}

func runDateTimeTool(_ tool.Context, _ map[string]any) (map[string]any, error) {
	now := time.Now()
	return map[string]any{
		"local": now.Format(time.RFC3339),
		"utc":   now.UTC().Format(time.RFC3339),
	}, nil
}

func runWebFetchTool(_ tool.Context, args webFetchArgs) (map[string]any, error) {
	url := strings.TrimSpace(args.URL)
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}
	if !strings.HasPrefix(strings.ToLower(url), "http://") && !strings.HasPrefix(strings.ToLower(url), "https://") {
		return nil, fmt.Errorf("only http and https URLs are supported")
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Aranea-Agent/1.0")
	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return nil, err
	}
	text := strings.Join(strings.Fields(string(body)), " ")
	if len([]rune(text)) > 6000 {
		runes := []rune(text)
		text = string(runes[:6000]) + "..."
	}
	return map[string]any{
		"url":          url,
		"status_code":  resp.StatusCode,
		"content_type": resp.Header.Get("Content-Type"),
		"text":         text,
	}, nil
}

func resolveWorkspacePath(rawPath string) (absPath string, relPath string, err error) {
	root, err := fileToolWorkspaceRoot()
	if err != nil {
		return "", "", err
	}
	input := strings.TrimSpace(rawPath)
	if input == "" || input == "." {
		input = "."
	}
	input = filepath.FromSlash(input)
	if !filepath.IsAbs(input) && filepath.Base(root) == "aranea" {
		parts := strings.Split(filepath.Clean(input), string(filepath.Separator))
		if len(parts) > 1 && strings.EqualFold(parts[0], "aranea") {
			input = filepath.Join(parts[1:]...)
		}
	}

	candidate := input
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate, err = filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", "", err
	}
	if rel == "." {
		return candidate, ".", nil
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("path %q is outside workspace sandbox", rawPath)
	}
	return candidate, filepath.ToSlash(rel), nil
}

func fileToolWorkspaceRoot() (string, error) {
	for _, key := range []string{"ARANEA_WORKSPACE_ROOT", "WORKSPACE_ROOT"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return filepath.Abs(filepath.Clean(value))
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	wd, err = filepath.Abs(wd)
	if err != nil {
		return "", err
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if hasDir(filepath.Join(dir, "backend")) && hasDir(filepath.Join(dir, "frontend")) {
			return dir, nil
		}
		aranea := filepath.Join(dir, "aranea")
		if hasDir(filepath.Join(aranea, "backend")) && hasDir(filepath.Join(aranea, "frontend")) {
			return aranea, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
	}
	return wd, nil
}

func hasDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
