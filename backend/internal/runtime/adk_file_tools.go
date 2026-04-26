package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
