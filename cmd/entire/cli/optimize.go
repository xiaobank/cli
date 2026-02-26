package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/entireio/cli/cmd/entire/cli/checkpoint"
	"github.com/entireio/cli/cmd/entire/cli/compression"
	"github.com/entireio/cli/cmd/entire/cli/logging"
	"github.com/entireio/cli/cmd/entire/cli/paths"
	"github.com/spf13/cobra"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func newOptimizeCmd() *cobra.Command {
	var dryRunFlag bool

	cmd := &cobra.Command{
		Use:   "optimize",
		Short: "Optimize stored checkpoint data",
		Long: `Compress existing uncompressed transcript data on the entire/checkpoints/v1 branch.

New checkpoints are automatically compressed with zstd. This command migrates
older uncompressed data to the compressed format, reducing storage size and
improving push/pull performance.

Default: dry run that shows what would be compressed and estimated savings.
With --apply, actually compresses the data.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runOptimize(cmd.OutOrStdout(), !dryRunFlag)
		},
	}

	cmd.Flags().BoolVar(&dryRunFlag, "apply", false, "Actually compress data (default: dry run)")

	return cmd
}

func runOptimize(w io.Writer, apply bool) error {
	logging.SetLogLevelGetter(GetLogLevel)
	if err := logging.Init(""); err == nil {
		defer logging.Close()
	}

	repoRoot, err := paths.WorktreeRoot()
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	repo, err := git.PlainOpen(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Get the metadata branch
	refName := plumbing.NewBranchReferenceName(paths.MetadataBranchName)
	ref, err := repo.Reference(refName, true)
	if err != nil {
		fmt.Fprintln(w, "No checkpoint data found (entire/checkpoints/v1 branch does not exist).")
		return nil
	}

	commit, err := repo.CommitObject(ref.Hash())
	if err != nil {
		return fmt.Errorf("failed to get commit: %w", err)
	}

	tree, err := commit.Tree()
	if err != nil {
		return fmt.Errorf("failed to get tree: %w", err)
	}

	// Walk the tree to find uncompressed transcript blobs
	entries := make(map[string]object.TreeEntry)
	if err := checkpoint.FlattenTree(repo, tree, "", entries); err != nil {
		return fmt.Errorf("failed to flatten tree: %w", err)
	}

	var changes []checkpoint.TreeChange
	var totalOriginalSize int64
	var totalCompressedSize int64
	var filesCompressed int

	for entryPath, entry := range entries {
		if !isUncompressedTranscript(entryPath) {
			continue
		}

		// Read the blob content
		blob, err := repo.BlobObject(entry.Hash)
		if err != nil {
			continue
		}

		reader, err := blob.Reader()
		if err != nil {
			continue
		}

		content := make([]byte, blob.Size)
		n, readErr := io.ReadFull(reader, content)
		_ = reader.Close()
		if readErr != nil && n == 0 {
			continue
		}
		content = content[:n]

		originalSize := int64(len(content))

		// Compress the content
		compressed, err := compression.Compress(content)
		if err != nil {
			continue
		}

		compressedSize := int64(len(compressed))
		totalOriginalSize += originalSize
		totalCompressedSize += compressedSize
		filesCompressed++

		if apply {
			// Create compressed blob
			blobHash, err := checkpoint.CreateBlobFromContent(repo, compressed)
			if err != nil {
				continue
			}

			// Delete old uncompressed entry
			changes = append(changes, checkpoint.TreeChange{
				Path:  entryPath,
				Entry: nil, // delete
			})

			// Add new compressed entry
			compressedPath := entryPath + paths.CompressedSuffix
			changes = append(changes, checkpoint.TreeChange{
				Path:  compressedPath,
				Entry: &object.TreeEntry{Mode: filemode.Regular, Hash: blobHash},
			})
		}
	}

	if filesCompressed == 0 {
		fmt.Fprintln(w, "All checkpoint data is already compressed. Nothing to optimize.")
		return nil
	}

	if !apply {
		fmt.Fprintf(w, "Dry run: found %d uncompressed transcript files\n", filesCompressed)
		fmt.Fprintf(w, "  Original size:   %s\n", formatSize(totalOriginalSize))
		fmt.Fprintf(w, "  Compressed size: %s\n", formatSize(totalCompressedSize))
		if totalOriginalSize > 0 {
			ratio := float64(totalOriginalSize) / float64(totalCompressedSize)
			savings := float64(totalOriginalSize-totalCompressedSize) / float64(totalOriginalSize) * 100
			fmt.Fprintf(w, "  Compression ratio: %.1fx (%.0f%% savings)\n", ratio, savings)
		}
		fmt.Fprintln(w, "\nRun with --apply to compress the data.")
		return nil
	}

	// Apply the changes
	newTreeHash, err := checkpoint.ApplyTreeChanges(repo, tree.Hash, changes)
	if err != nil {
		return fmt.Errorf("failed to apply tree changes: %w", err)
	}

	// Create new commit
	authorName, authorEmail := checkpoint.GetGitAuthorFromRepo(repo)
	now := plumbing.NewHashReference(refName, plumbing.ZeroHash) // placeholder
	_ = now

	commitObj := &object.Commit{
		Author:    object.Signature{Name: authorName, Email: authorEmail},
		Committer: object.Signature{Name: authorName, Email: authorEmail},
		Message:   fmt.Sprintf("Optimize: compress %d transcript files\n", filesCompressed),
		TreeHash:  newTreeHash,
		ParentHashes: []plumbing.Hash{
			ref.Hash(),
		},
	}

	newCommitHash, err := storeCommitObject(repo, commitObj)
	if err != nil {
		return fmt.Errorf("failed to create commit: %w", err)
	}

	// Update branch ref
	newRef := plumbing.NewHashReference(refName, newCommitHash)
	if err := repo.Storer.SetReference(newRef); err != nil {
		return fmt.Errorf("failed to update branch reference: %w", err)
	}

	fmt.Fprintf(w, "Compressed %d transcript files\n", filesCompressed)
	fmt.Fprintf(w, "  Original size:   %s\n", formatSize(totalOriginalSize))
	fmt.Fprintf(w, "  Compressed size: %s\n", formatSize(totalCompressedSize))
	if totalOriginalSize > 0 {
		ratio := float64(totalOriginalSize) / float64(totalCompressedSize)
		savings := float64(totalOriginalSize-totalCompressedSize) / float64(totalOriginalSize) * 100
		fmt.Fprintf(w, "  Compression ratio: %.1fx (%.0f%% savings)\n", ratio, savings)
	}
	return nil
}

// isUncompressedTranscript returns true if the path is an uncompressed transcript file
// (full.jsonl or agent-*.jsonl, not already compressed with .zst).
func isUncompressedTranscript(path string) bool {
	if strings.HasSuffix(path, paths.CompressedSuffix) {
		return false
	}
	base := pathBase(path)
	if base == paths.TranscriptFileName {
		return true
	}
	// Match full.jsonl.001, full.jsonl.002, etc.
	if strings.HasPrefix(base, paths.TranscriptFileName+".") {
		return true
	}
	// Match agent-*.jsonl
	if strings.HasPrefix(base, "agent-") && strings.HasSuffix(base, ".jsonl") {
		return true
	}
	return false
}

// pathBase returns the last component of a slash-separated path.
func pathBase(path string) string {
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// storeCommitObject stores a commit object in the repository.
func storeCommitObject(repo *git.Repository, c *object.Commit) (plumbing.Hash, error) {
	obj := repo.Storer.NewEncodedObject()
	if err := c.Encode(obj); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("failed to encode commit: %w", err)
	}
	hash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("failed to store commit: %w", err)
	}
	return hash, nil
}

// formatSize formats a byte count as a human-readable string.
func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
