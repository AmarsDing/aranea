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
			description: "Read a UTF-8 text file inside the workspace sandbox. Args: { path }.",
			factory: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "read_file",
					Description: "Read a UTF-8 text file inside the workspace sandbox. The path may be relative to the project root.",
				}, runReadFileTool)
			},
		},
		{
			name:        "list_files",
			description: "List files and directories inside a workspace directory. Args: { path }.",
			factory: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "list_files",
					Description: "List files and directories inside a workspace directory. The path may be empty or relative to the project root.",
				}, runListFilesTool)
			},
		},
		{
			name:        "write_file",
			description: "Create or overwrite a file inside the workspace sandbox. Args: { path, content }.",
			factory: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "write_file",
					Description: "Create or overwrite a file inside the workspace sandbox. Parent directories are created automatically.",
				}, runWriteFileTool)
			},
		},
		{
			name:        "edit_file",
			description: "Replace exactly one text occurrence in an existing workspace file. Args: { path, old_string, new_string }.",
			factory: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "edit_file",
					Description: "Replace exactly one text occurrence in an existing workspace file. Fails if old_string is missing or ambiguous.",
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
					Description: "Return the current local and UTC time.",
				}, runDateTimeTool)
			},
		},
		{
			name: "web_fetch",
			factory: func() (tool.Tool, error) {
				return functiontool.New(functiontool.Config{
					Name:        "web_fetch",
					Description: "Fetch a public HTTP/HTTPS URL and return a short text preview. Args: { url }.",
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

func runtimeProfileAllows(profile string, name string) bool {
	switch strings.TrimSpace(profile) {
	case "minimal":
		return name == "datetime"
	case "safe":
		return name == "datetime" || name == "read_file" || name == "list_files" || name == "web_fetch"
	case "coding":
		return name == "datetime" || name == "read_file" || name == "write_file" || name == "list_files" || name == "edit_file" || name == "web_fetch"
	case "research":
		return name == "datetime" || name == "read_file" || name == "list_files" || name == "web_fetch"
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
		return nil, fmt.Errorf("path %q is a directory", rel)
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
