package skill

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"arenea/backend/cmd/aranea/internal/apiclient"
	"arenea/backend/cmd/aranea/internal/output"
	"arenea/backend/internal/domain"
)

// newInstallCmd implements `aranea skill install <url>`. It supports git
// repositories (github.com / gitlab / generic .git URLs) and remote zip
// files. The flow follows 前端/25 cli.md §6:
//
//  1. parse the URL into a source descriptor (scheme + ref + subpath)
//  2. clone or download into a temp directory
//  3. locate the SKILL.md root (single skill or pickable subdir)
//  4. run a small set of local pre-validations (front-matter required)
//  5. zip the chosen directory and POST it to /api/v1/skills/import
//  6. poll the returned job until it leaves the `validating` phase
//  7. resolve any conflicts interactively (skip / keep / refine)
//  8. POST /apply to commit the surviving candidates
func newInstallCmd(g *apiclient.GlobalContext) *cobra.Command {
	var (
		ref          string
		subpath      string
		dryRun       bool
		conflictMode string
		keep         string
	)
	cmd := &cobra.Command{
		Use:   "install <url>",
		Short: "Install a skill from a git URL or remote zip",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := parseSource(args[0], ref, subpath)
			if err != nil {
				return err
			}
			tmp, err := os.MkdirTemp("", "aranea-skill-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmp)

			if !output.Quiet() {
				fmt.Fprintf(cmd.OutOrStdout(), "fetching %s ...\n", src.URL)
			}
			workdir, err := fetchSource(cmd.Context(), src, tmp)
			if err != nil {
				return err
			}
			skillRoot, err := locateSkillRoot(workdir, src.Subpath)
			if err != nil {
				return err
			}
			if !output.Quiet() {
				fmt.Fprintf(cmd.OutOrStdout(), "skill root: %s\n", skillRoot)
			}
			if err := preValidate(skillRoot); err != nil {
				return fmt.Errorf("local validation failed: %w", err)
			}
			if dryRun {
				output.Success(cmd.OutOrStdout(), "dry-run ok: skill is well-formed")
				return nil
			}
			zipBuf, err := zipDir(skillRoot)
			if err != nil {
				return err
			}
			job, err := uploadAndWait(cmd.Context(), g, zipBuf, filepath.Base(skillRoot)+".zip")
			if err != nil {
				return err
			}
			if !output.Quiet() {
				fmt.Fprintf(cmd.OutOrStdout(), "import job %s: %s (%d candidates, %d conflicts)\n",
					job.JobID, job.ValidationStatus, len(job.Candidates), len(job.ConflictGroups))
			}
			decisions, err := resolveConflicts(cmd, g, job, conflictMode, keep)
			if err != nil {
				return err
			}
			result, err := applyImport(cmd.Context(), g, job.JobID, decisions)
			if err != nil {
				return err
			}
			if !output.Quiet() {
				fmt.Fprintf(cmd.OutOrStdout(), "created skills: %v\n", result.CreatedSkillIDs)
				if len(result.SkippedCandidateIDs) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "skipped: %v\n", result.SkippedCandidateIDs)
				}
			}
			output.Success(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().StringVar(&ref, "ref", "", "Git ref to checkout (branch / tag / commit)")
	cmd.Flags().StringVar(&subpath, "subpath", "", "Subdirectory inside the repo containing SKILL.md")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Run local validation only; do not upload")
	cmd.Flags().StringVar(&conflictMode, "on-conflict", "ask", "ask|skip|keep|refine — default behavior when a conflict is detected")
	cmd.Flags().StringVar(&keep, "keep", "incoming", "incoming|existing — value used when --on-conflict=keep")
	return cmd
}

func newImportCmd(g *apiclient.GlobalContext) *cobra.Command {
	var conflictMode, keep string
	cmd := &cobra.Command{
		Use:   "import <zip-path>",
		Short: "Import a skill from a local zip file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer f.Close()
			job, err := uploadAndWait(cmd.Context(), g, f, filepath.Base(args[0]))
			if err != nil {
				return err
			}
			decisions, err := resolveConflicts(cmd, g, job, conflictMode, keep)
			if err != nil {
				return err
			}
			result, err := applyImport(cmd.Context(), g, job.JobID, decisions)
			if err != nil {
				return err
			}
			output.Success(cmd.OutOrStdout(), result.Message)
			return nil
		},
	}
	cmd.Flags().StringVar(&conflictMode, "on-conflict", "ask", "ask|skip|keep|refine")
	cmd.Flags().StringVar(&keep, "keep", "incoming", "incoming|existing")
	return cmd
}

// source describes a normalised skill source.
type source struct {
	Kind    string // git | zip | local
	URL     string
	Ref     string
	Subpath string
}

// parseSource canonicalises common URL shapes:
//
//   - github.com/<owner>/<repo>            → git, https://github.com/<owner>/<repo>.git
//   - github.com/<owner>/<repo>/tree/<ref>/<sub> → git + ref + subpath
//   - https://...                          → git or zip based on suffix
//   - file:// or local paths               → local
func parseSource(rawURL, refOverride, subpathOverride string) (source, error) {
	if !strings.Contains(rawURL, "://") && !strings.HasPrefix(rawURL, "github.com") &&
		!strings.HasPrefix(rawURL, "gitlab.com") {
		// treat as a local path
		abs, err := filepath.Abs(rawURL)
		if err != nil {
			return source{}, err
		}
		return source{Kind: "local", URL: abs, Subpath: subpathOverride}, nil
	}
	if strings.HasPrefix(rawURL, "github.com/") || strings.HasPrefix(rawURL, "gitlab.com/") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return source{}, fmt.Errorf("invalid URL: %w", err)
	}
	if strings.HasSuffix(u.Path, ".zip") {
		return source{Kind: "zip", URL: u.String(), Subpath: subpathOverride}, nil
	}
	src := source{Kind: "git", Ref: refOverride, Subpath: subpathOverride}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if (u.Host == "github.com" || u.Host == "gitlab.com") && len(parts) >= 2 {
		owner, repo := parts[0], strings.TrimSuffix(parts[1], ".git")
		src.URL = fmt.Sprintf("https://%s/%s/%s.git", u.Host, owner, repo)
		// Detect /tree/<ref>/<sub...> from a browse URL.
		if len(parts) >= 4 && parts[2] == "tree" {
			if src.Ref == "" {
				src.Ref = parts[3]
			}
			if src.Subpath == "" && len(parts) > 4 {
				src.Subpath = strings.Join(parts[4:], "/")
			}
		}
		return src, nil
	}
	src.URL = u.String()
	return src, nil
}

// fetchSource clones or downloads the source into dir and returns the
// directory that should be searched for SKILL.md files.
func fetchSource(ctx context.Context, src source, dir string) (string, error) {
	switch src.Kind {
	case "local":
		return src.URL, nil
	case "git":
		args := []string{"clone", "--depth", "1"}
		if src.Ref != "" {
			args = append(args, "--branch", src.Ref)
		}
		args = append(args, src.URL, dir)
		cmd := exec.CommandContext(ctx, "git", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git clone: %w (%s)", err, strings.TrimSpace(stderr.String()))
		}
		return dir, nil
	case "zip":
		return downloadAndExtractZip(ctx, src.URL, dir)
	}
	return "", fmt.Errorf("unsupported source kind: %s", src.Kind)
}

// downloadAndExtractZip streams a remote zip into dir/source.zip and
// expands it into dir/extracted. We use a 50MB cap to keep the worst
// case sane; archives larger than that should be brought in over git.
// The function tolerates GitHub-style archives that wrap content in a
// `<repo>-<ref>/` prefix because locateSkillRoot walks the whole tree.
func downloadAndExtractZip(ctx context.Context, rawURL, dir string) (string, error) {
	const maxZipBytes = 50 * 1024 * 1024
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "aranea-cli/skill-install")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download zip: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("download zip: HTTP %d", resp.StatusCode)
	}
	zipPath := filepath.Join(dir, "source.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(out, io.LimitReader(resp.Body, maxZipBytes+1)); err != nil {
		out.Close()
		return "", fmt.Errorf("write zip: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	info, err := os.Stat(zipPath)
	if err != nil {
		return "", err
	}
	if info.Size() > maxZipBytes {
		return "", fmt.Errorf("downloaded archive exceeds %dMB cap", maxZipBytes/(1024*1024))
	}
	extractDir := filepath.Join(dir, "extracted")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return "", err
	}
	if err := unzipInto(zipPath, extractDir); err != nil {
		return "", err
	}
	return extractDir, nil
}

// unzipInto expands archivePath into destDir while guarding against
// path traversal (Zip Slip) and refusing oversized members. Each entry
// must resolve inside destDir; symlink and device entries are skipped.
func unzipInto(archivePath, destDir string) error {
	const maxFileBytes = 20 * 1024 * 1024
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()
	cleanDest := filepath.Clean(destDir) + string(os.PathSeparator)
	for _, f := range zr.File {
		if !f.Mode().IsRegular() && !f.FileInfo().IsDir() {
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(f.Name))
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDest) {
			return fmt.Errorf("zip entry %q escapes destination", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if f.UncompressedSize64 > maxFileBytes {
			return fmt.Errorf("zip entry %q exceeds %dMB", f.Name, maxFileBytes/(1024*1024))
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %q: %w", f.Name, err)
		}
		w, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(w, io.LimitReader(rc, int64(maxFileBytes)+1)); err != nil {
			rc.Close()
			w.Close()
			return fmt.Errorf("extract %q: %w", f.Name, err)
		}
		rc.Close()
		if err := w.Close(); err != nil {
			return err
		}
	}
	return nil
}

// locateSkillRoot returns the directory that should be zipped. If
// subpath is set we trust the caller. Otherwise we walk the tree
// looking for SKILL.md files: 0 → error, 1 → use it, >1 → error so the
// user is forced to pick with --subpath.
func locateSkillRoot(root, subpath string) (string, error) {
	if subpath != "" {
		full := filepath.Join(root, filepath.FromSlash(subpath))
		if _, err := os.Stat(filepath.Join(full, "SKILL.md")); err != nil {
			return "", fmt.Errorf("SKILL.md not found in %s", full)
		}
		return full, nil
	}
	matches := []string{}
	if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == ".venv" {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == "SKILL.md" {
			matches = append(matches, filepath.Dir(p))
		}
		return nil
	}); err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", errors.New("no SKILL.md found in source; pass --subpath to point at a directory")
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("multiple SKILL.md candidates: %v — pick one with --subpath", matches)
	}
}

// preValidate runs cheap local checks before we waste a network round
// trip. We require SKILL.md, a non-empty front-matter description, and
// the file to be smaller than ~512KB.
func preValidate(root string) error {
	body, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil {
		return err
	}
	if len(body) > 512*1024 {
		return fmt.Errorf("SKILL.md too large: %d bytes (limit 512KB)", len(body))
	}
	text := string(body)
	if !strings.HasPrefix(text, "---") {
		return errors.New("SKILL.md must begin with a YAML front-matter delimiter (---)")
	}
	end := strings.Index(text[3:], "\n---")
	if end < 0 {
		return errors.New("SKILL.md front-matter must be terminated with `---`")
	}
	front := text[3 : 3+end]
	if !strings.Contains(front, "description") {
		return errors.New("SKILL.md front-matter must define `description`")
	}
	return nil
}

// zipDir packs root (relative paths only) into an in-memory zip archive
// suitable for streaming to the import endpoint.
func zipDir(root string) (io.Reader, error) {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	prefix := filepath.Base(root) + "/"
	if err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		w, err := zw.Create(prefix + filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(w, f)
		return err
	}); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf, nil
}

// uploadAndWait POSTs the zip and polls the resulting job until the
// backend declares it ready_to_apply or returns an error status.
func uploadAndWait(ctx context.Context, g *apiclient.GlobalContext, body io.Reader, fileName string) (domain.SkillImportJob, error) {
	var startResp struct {
		JobID string `json:"job_id"`
	}
	if err := g.Client().PostMultipart(ctx, "/api/v1/skills/import", "file", fileName, body, nil, &startResp); err != nil {
		return domain.SkillImportJob{}, err
	}
	return waitForJob(ctx, g, startResp.JobID)
}

func waitForJob(ctx context.Context, g *apiclient.GlobalContext, jobID string) (domain.SkillImportJob, error) {
	deadline := time.Now().Add(5 * time.Minute)
	for {
		var job domain.SkillImportJob
		if err := g.Client().Get(ctx, "/api/v1/skills/import/"+url.PathEscape(jobID), nil, &job); err != nil {
			return domain.SkillImportJob{}, err
		}
		switch job.Status {
		case "ready_to_apply", "applied", "completed", "needs_review":
			return job, nil
		case "failed", "cancelled":
			return job, fmt.Errorf("import job %s ended with status %s: %s", jobID, job.Status, job.Message)
		}
		if time.Now().After(deadline) {
			return job, fmt.Errorf("timeout waiting for import job %s (last status: %s)", jobID, job.Status)
		}
		select {
		case <-ctx.Done():
			return job, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}

// resolveConflicts builds the apply decision list. Modes:
//
//   - skip   → drop every candidate that is part of a conflict group
//   - keep   → either keep the existing skill (no candidate created) or
//     keep the incoming candidate (the default sub-flag value)
//   - refine → call /refine on each conflict group then accept the merged
//     payload as a new candidate
//   - ask    → drop into an interactive REPL one group at a time
func resolveConflicts(cmd *cobra.Command, g *apiclient.GlobalContext, job domain.SkillImportJob, mode, keep string) ([]domain.SkillImportDecision, error) {
	decisions := make([]domain.SkillImportDecision, 0, len(job.Candidates))
	conflicted := map[string]string{}
	for _, group := range job.ConflictGroups {
		for _, cid := range group.CandidateIDs {
			conflicted[cid] = group.GroupID
		}
	}
	for _, c := range job.Candidates {
		if _, ok := conflicted[c.CandidateID]; !ok {
			decisions = append(decisions, domain.SkillImportDecision{CandidateID: c.CandidateID, Action: "create"})
		}
	}
	for _, group := range job.ConflictGroups {
		decision, err := decideGroup(cmd, g, job.JobID, group, mode, keep)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

func decideGroup(cmd *cobra.Command, g *apiclient.GlobalContext, jobID string, group domain.SkillConflictGroup, mode, keep string) (domain.SkillImportDecision, error) {
	if mode == "ask" {
		mode = askConflictMode(cmd, group)
	}
	switch mode {
	case "skip":
		return domain.SkillImportDecision{GroupID: group.GroupID, Action: "skip"}, nil
	case "keep":
		if keep == "existing" {
			return domain.SkillImportDecision{GroupID: group.GroupID, Action: "keep_existing"}, nil
		}
		return domain.SkillImportDecision{GroupID: group.GroupID, Action: "keep_incoming"}, nil
	case "refine":
		var refined domain.SkillRefineResult
		req := domain.SkillRefineRequest{Instructions: "Merge into a single canonical skill"}
		path := fmt.Sprintf("/api/v1/skills/import/%s/conflict-groups/%s/refine", url.PathEscape(jobID), url.PathEscape(group.GroupID))
		if err := g.Client().Post(cmd.Context(), path, req, &refined); err != nil {
			return domain.SkillImportDecision{}, err
		}
		return domain.SkillImportDecision{
			GroupID:           group.GroupID,
			Action:            "merge",
			MergedName:        refined.MergedName,
			MergedDescription: refined.MergedDescription,
			MergedBody:        refined.MergedBody,
			MergedTags:        refined.MergedTags,
		}, nil
	}
	return domain.SkillImportDecision{GroupID: group.GroupID, Action: "skip"}, nil
}

func askConflictMode(cmd *cobra.Command, group domain.SkillConflictGroup) string {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "\nconflict %s (similarity %.2f, risk %s):\n", group.GroupID, group.HighestSimilarityScore, group.Metrics.ConflictRisk)
	for _, src := range group.ExistingSkills {
		fmt.Fprintf(w, "  existing: %s (%s)\n", src.Name, src.Slug)
	}
	fmt.Fprintf(w, "  reason  : %s\n", group.Reason)
	fmt.Fprint(w, "  choose [k]eep-incoming / [e]xisting / [r]efine / [s]kip: ")
	r := bufio.NewReader(os.Stdin)
	answer, _ := r.ReadString('\n')
	switch strings.TrimSpace(strings.ToLower(answer)) {
	case "k", "keep", "keep-incoming":
		return "keep"
	case "e", "existing", "keep-existing":
		return "keep"
	case "r", "refine":
		return "refine"
	default:
		return "skip"
	}
}

func applyImport(ctx context.Context, g *apiclient.GlobalContext, jobID string, decisions []domain.SkillImportDecision) (domain.SkillImportApplyResult, error) {
	var result domain.SkillImportApplyResult
	body := domain.SkillImportApplyRequest{Decisions: decisions}
	if err := g.Client().Post(ctx, fmt.Sprintf("/api/v1/skills/import/%s/apply", url.PathEscape(jobID)), body, &result); err != nil {
		return domain.SkillImportApplyResult{}, err
	}
	return result, nil
}

// confirm is shared by every mutating skill command. When --yes / -y is
// set we never prompt; otherwise we read a y/N answer from stdin.
func confirm(cmd *cobra.Command, g *apiclient.GlobalContext, prompt string) bool {
	if g.Yes {
		return true
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N] ", prompt)
	r := bufio.NewReader(os.Stdin)
	answer, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	}
	return false
}

// safeJoin joins a base URL with the path while ensuring no traversal.
func safeJoin(base, p string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path.Clean(p), "/")
}
