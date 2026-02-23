package main

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed index.html
var staticFS embed.FS

// --- API response types ---

// CheckpointResponse is what /api/checkpoints returns per commit.
type CheckpointResponse struct {
	Hash         string            `json:"hash"`
	ShortHash    string            `json:"short_hash"`
	Subject      string            `json:"subject"`
	Date         string            `json:"date"`
	CheckpointID string            `json:"checkpoint_id"`
	RootMeta     json.RawMessage   `json:"root_metadata"`
	Sessions     []SessionOnBranch `json:"sessions"`
}

// SessionOnBranch is a session stored on entire/checkpoints/v1.
type SessionOnBranch struct {
	Index    int             `json:"index"`
	Metadata json.RawMessage `json:"metadata"`
	Files    []string        `json:"files"`
}

// --- Worktree hash for shadow branch filtering ---

// worktreeHash returns the 6-char hash suffix used in shadow branch names
// for the current working directory. Matches the CLI's checkpoint.HashWorktreeID().
func worktreeHash() string {
	// Determine worktreeID: empty for main worktree, name for linked worktrees.
	gitPath := filepath.Join(".", ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return hashID("")
	}
	if info.IsDir() {
		// Main worktree
		return hashID("")
	}
	// Linked worktree: .git file contains "gitdir: .../.git/worktrees/<name>"
	content, err := os.ReadFile(gitPath)
	if err != nil {
		return hashID("")
	}
	line := strings.TrimSpace(string(content))
	const marker = ".git/worktrees/"
	_, id, found := strings.Cut(line, marker)
	if !found {
		return hashID("")
	}
	return hashID(strings.TrimSuffix(id, "/"))
}

func hashID(id string) string {
	h := sha256.Sum256([]byte(id))
	return hex.EncodeToString(h[:])[:6]
}

// --- Cached git common dir ---

var (
	gitCommonDirOnce sync.Once
	gitCommonDirVal  string
)

func gitCommonDir() string {
	gitCommonDirOnce.Do(func() {
		out, err := exec.Command("git", "rev-parse", "--git-common-dir").Output()
		if err != nil {
			log.Printf("warning: git rev-parse --git-common-dir failed: %v", err)
			gitCommonDirVal = ".git"
			return
		}
		gitCommonDirVal = strings.TrimSpace(string(out))
	})
	return gitCommonDirVal
}

func currentBranch() string {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// mainBranch returns the name of the main/master branch.
func mainBranch() string {
	// Try common names
	for _, name := range []string{"main", "master"} {
		if err := exec.Command("git", "rev-parse", "--verify", "--quiet", "refs/heads/"+name).Run(); err == nil {
			return name
		}
	}
	return ""
}

// gitLog returns commits with checkpoint trailers, scoped to the current branch only.
func gitLog(limit int) []CheckpointResponse {
	format := "%H%x00%s%x00%aI%x00%(trailers:key=Entire-Checkpoint,valueonly)"

	// Only show commits on the current branch (not in main).
	// If we ARE on main, fall back to showing recent commits.
	branch := currentBranch()
	base := mainBranch()
	var args []string
	if base != "" && branch != base {
		args = []string{"log", fmt.Sprintf("--format=%s", format), base + "..HEAD"}
	} else {
		args = []string{"log", fmt.Sprintf("--format=%s", format), fmt.Sprintf("-%d", limit)}
	}

	out, err := exec.Command("git", args...).Output()
	if err != nil {
		log.Printf("git log failed: %v", err)
		return nil
	}

	var results []CheckpointResponse
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 4)
		if len(parts) < 4 {
			continue
		}
		cpID := strings.TrimSpace(parts[3])
		if cpID == "" {
			continue
		}
		hash := parts[0]
		short := hash
		if len(short) > 7 {
			short = short[:7]
		}
		results = append(results, CheckpointResponse{
			Hash:         hash,
			ShortHash:    short,
			Subject:      parts[1],
			Date:         parts[2],
			CheckpointID: cpID,
		})
	}
	return results
}

// cpBasePath returns the sharded base path for a checkpoint ID on the metadata branch.
func cpBasePath(cpID string) string {
	if len(cpID) < 3 {
		return cpID
	}
	return cpID[:2] + "/" + cpID[2:]
}

// gitShowRaw reads a blob from a git ref and returns it as-is.
func gitShowRaw(ref string) ([]byte, error) {
	out, err := exec.Command("git", "show", ref).Output()
	if err != nil {
		return nil, err
	}
	return out, nil
}

// listTreePaths lists all file paths under a given tree path on a branch.
func listTreePaths(branch, dirPath string) []string {
	// git ls-tree -r --name-only <branch>:<dirPath>
	ref := branch + ":" + dirPath
	out, err := exec.Command("git", "ls-tree", "-r", "--name-only", ref).Output()
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			// paths are relative to dirPath, prepend it
			paths = append(paths, dirPath+"/"+line)
		}
	}
	return paths
}

// readCheckpointSessions reads all per-session data for a checkpoint.
func readCheckpointSessions(cpID string) []SessionOnBranch {
	base := cpBasePath(cpID)
	var sessions []SessionOnBranch
	for i := 0; i < 20; i++ {
		sessionDir := fmt.Sprintf("%s/%d", base, i)
		metaRef := fmt.Sprintf("entire/checkpoints/v1:%s/metadata.json", sessionDir)
		raw, err := gitShowRaw(metaRef)
		if err != nil {
			break
		}
		files := listTreePaths("entire/checkpoints/v1", sessionDir)
		sessions = append(sessions, SessionOnBranch{
			Index:    i,
			Metadata: json.RawMessage(raw),
			Files:    files,
		})
	}
	return sessions
}

// readRootMeta reads the root metadata.json for a checkpoint.
func readRootMeta(cpID string) json.RawMessage {
	ref := fmt.Sprintf("entire/checkpoints/v1:%s/metadata.json", cpBasePath(cpID))
	raw, err := gitShowRaw(ref)
	if err != nil {
		return nil
	}
	return json.RawMessage(raw)
}

// readSessionStateFiles reads all .json files from .git/entire-sessions/ as raw JSON.
func readSessionStateFiles() []json.RawMessage {
	dir := filepath.Join(gitCommonDir(), "entire-sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var sessions []json.RawMessage
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		// Validate it's proper JSON
		if !json.Valid(data) {
			continue
		}
		sessions = append(sessions, json.RawMessage(data))
	}
	return sessions
}

// --- Handlers ---

func handleCheckpoints(w http.ResponseWriter, _ *http.Request) {
	checkpoints := gitLog(50)
	if checkpoints == nil {
		checkpoints = []CheckpointResponse{}
	}

	var wg sync.WaitGroup
	for i := range checkpoints {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cp := &checkpoints[idx]
			cp.RootMeta = readRootMeta(cp.CheckpointID)
			cp.Sessions = readCheckpointSessions(cp.CheckpointID)
		}(i)
	}
	wg.Wait()

	writeJSON(w, checkpoints)
}

func handleSessions(w http.ResponseWriter, _ *http.Request) {
	sessions := readSessionStateFiles()
	if sessions == nil {
		sessions = []json.RawMessage{}
	}
	writeJSON(w, sessions)
}

func handleBranch(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"branch": currentBranch()})
}

// handleBlob serves raw file contents from the entire/checkpoints/v1 branch.
// GET /api/blob?path=<shard>/<rest>/<idx>/full.jsonl
func handleBlob(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "missing path parameter", http.StatusBadRequest)
		return
	}
	// Sanitize: only allow paths under the checkpoints tree
	if strings.Contains(path, "..") {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	ref := "entire/checkpoints/v1:" + path
	data, err := gitShowRaw(ref)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Detect content type
	if strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".jsonl") {
		w.Header().Set("Content-Type", "application/json")
	} else if strings.HasSuffix(path, ".md") {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.Write(data)
}

// handleShadowBranches lists entire/* shadow branches with their latest commit info.
func handleShadowBranches(w http.ResponseWriter, _ *http.Request) {
	// List branches matching entire/* but not entire/checkpoints/*
	out, err := exec.Command("git", "for-each-ref", "--format=%(refname:short)%00%(objectname:short)%00%(committerdate:iso-strict)%00%(subject)", "refs/heads/entire/").Output()
	if err != nil {
		writeJSON(w, []any{})
		return
	}

	type BranchInfo struct {
		Name      string `json:"name"`
		ShortHash string `json:"short_hash"`
		Date      string `json:"date"`
		Subject   string `json:"subject"`
	}

	// Only show shadow branches for the current working directory.
	// Shadow branch format: entire/<commit[:7]>-<worktreeHash[:6]>
	wtHash := worktreeHash()
	suffix := "-" + wtHash

	var branches []BranchInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\x00", 4)
		if len(parts) < 4 {
			continue
		}
		name := parts[0]
		// Skip the checkpoints branch
		if strings.HasPrefix(name, "entire/checkpoints") {
			continue
		}
		// Only keep branches whose name ends with our worktree hash
		if !strings.HasSuffix(name, suffix) {
			continue
		}
		branches = append(branches, BranchInfo{
			Name:      name,
			ShortHash: parts[1],
			Date:      parts[2],
			Subject:   parts[3],
		})
	}
	if branches == nil {
		branches = []BranchInfo{}
	}
	writeJSON(w, branches)
}

// handleShadowTree lists files on a shadow branch at a given path.
func handleShadowTree(w http.ResponseWriter, r *http.Request) {
	branch := r.URL.Query().Get("branch")
	path := r.URL.Query().Get("path")
	if branch == "" {
		http.Error(w, "missing branch parameter", http.StatusBadRequest)
		return
	}
	if strings.Contains(branch, "..") || strings.Contains(path, "..") {
		http.Error(w, "invalid parameter", http.StatusBadRequest)
		return
	}

	ref := branch
	if path != "" {
		ref = branch + ":" + path
	}
	out, err := exec.Command("git", "ls-tree", "-r", "--name-only", ref).Output()
	if err != nil {
		writeJSON(w, []string{})
		return
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			if path != "" {
				files = append(files, path+"/"+line)
			} else {
				files = append(files, line)
			}
		}
	}
	if files == nil {
		files = []string{}
	}
	writeJSON(w, files)
}

// handleShadowBlob reads a file from a shadow branch.
func handleShadowBlob(w http.ResponseWriter, r *http.Request) {
	branch := r.URL.Query().Get("branch")
	path := r.URL.Query().Get("path")
	if branch == "" || path == "" {
		http.Error(w, "missing branch/path parameter", http.StatusBadRequest)
		return
	}
	if strings.Contains(branch, "..") || strings.Contains(path, "..") {
		http.Error(w, "invalid parameter", http.StatusBadRequest)
		return
	}

	ref := branch + ":" + path
	data, err := gitShowRaw(ref)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".jsonl") {
		w.Header().Set("Content-Type", "application/json")
	} else if strings.HasSuffix(path, ".md") {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	w.Write(data)
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(buf.Bytes())
}

func main() {
	port := flag.Int("port", 8080, "HTTP server port")
	flag.Parse()

	if err := exec.Command("git", "rev-parse", "--git-dir").Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error: not a git repository. Run from within a git repo.")
		os.Exit(1)
	}

	http.HandleFunc("/api/checkpoints", handleCheckpoints)
	http.HandleFunc("/api/sessions", handleSessions)
	http.HandleFunc("/api/branch", handleBranch)
	http.HandleFunc("/api/blob", handleBlob)
	http.HandleFunc("/api/shadow-branches", handleShadowBranches)
	http.HandleFunc("/api/shadow-tree", handleShadowTree)
	http.HandleFunc("/api/shadow-blob", handleShadowBlob)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := staticFS.ReadFile("index.html")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	addr := fmt.Sprintf(":%d", *port)
	fmt.Fprintf(os.Stderr, "Checkpoint Viewer listening on http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
