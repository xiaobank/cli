package checkpoint_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entireio/cli/cmd/entire/cli/agent"
	"github.com/entireio/cli/cmd/entire/cli/benchutil"
	"github.com/entireio/cli/cmd/entire/cli/checkpoint"
	"github.com/entireio/cli/cmd/entire/cli/checkpoint/id"
	"github.com/entireio/cli/cmd/entire/cli/compression"
	"github.com/entireio/cli/cmd/entire/cli/paths"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// generateBenchJSONL creates a realistic JSONL transcript of the given approximate size.
func generateBenchJSONL(targetSize int) []byte {
	var buf strings.Builder
	templates := []string{
		`{"type":"human","message":{"id":"msg_%06d","content":"Please implement the feature for handling user authentication"}}`,
		`{"type":"assistant","message":{"id":"msg_%06d","content":"I'll implement the authentication feature. Let me start by creating the middleware...","model":"claude-sonnet-4-5-20250514","stop_reason":"end_turn","usage":{"input_tokens":1234,"output_tokens":5678}}}`,
		`{"type":"tool_use","id":"toolu_%06d","name":"Write","input":{"file_path":"/src/auth/middleware.go","content":"package auth\n\nfunc Middleware() {}"},"output":"File written successfully"}`,
		`{"type":"tool_result","id":"toolu_%06d","content":"Successfully wrote 45 bytes to /src/auth/middleware.go"}`,
	}
	i := 0
	for buf.Len() < targetSize {
		line := fmt.Sprintf(templates[i%len(templates)], i)
		buf.WriteString(line)
		buf.WriteByte('\n')
		i++
	}
	return []byte(buf.String())
}

// BenchmarkWriteTranscript benchmarks writing a transcript to the metadata branch
// with and without compression.
func BenchmarkWriteTranscript(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
	}

	for _, s := range sizes {
		transcript := generateBenchJSONL(s.size)
		b.Run(s.name, func(b *testing.B) {
			repo := benchutil.NewBenchRepo(b, benchutil.RepoOpts{FileCount: 5, FileSizeLines: 10})
			sessionID := repo.CreateSessionState(b, benchutil.SessionOpts{})

			metadataDir := paths.SessionMetadataDirFromSessionID(sessionID)
			metadataDirAbs := filepath.Join(repo.Dir, metadataDir)
			if err := os.MkdirAll(metadataDirAbs, 0o755); err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(metadataDirAbs, paths.TranscriptFileName), transcript, 0o644); err != nil {
				b.Fatal(err)
			}

			gitRepo, err := gogit.PlainOpen(repo.Dir)
			if err != nil {
				b.Fatal(err)
			}
			store := checkpoint.NewGitStore(gitRepo)

			b.SetBytes(int64(len(transcript)))
			b.ResetTimer()

			for i := range b.N {
				cpID := id.MustCheckpointID(fmt.Sprintf("%012x", i))
				err := store.WriteCommitted(context.Background(), checkpoint.WriteCommittedOptions{
					CheckpointID:     cpID,
					SessionID:        sessionID,
					Agent:            agent.AgentTypeClaudeCode,
					Transcript:       transcript,
					CheckpointsCount: 1,
					AuthorName:       "Bench",
					AuthorEmail:      "bench@test.com",
				})
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkReadCompressedBlob benchmarks reading and decompressing a transcript blob
// from a git repository, which is the core cost of the read path.
func BenchmarkReadCompressedBlob(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}

	for _, s := range sizes {
		transcript := generateBenchJSONL(s.size)
		b.Run(s.name, func(b *testing.B) {
			repo := benchutil.NewBenchRepo(b, benchutil.RepoOpts{FileCount: 5, FileSizeLines: 10})
			gitRepo, err := gogit.PlainOpen(repo.Dir)
			if err != nil {
				b.Fatal(err)
			}

			// Store compressed blob
			compressed, err := compression.Compress(transcript)
			if err != nil {
				b.Fatal(err)
			}
			blobHash, err := checkpoint.CreateBlobFromContent(gitRepo, compressed)
			if err != nil {
				b.Fatal(err)
			}

			b.SetBytes(int64(len(transcript)))
			b.ResetTimer()

			for b.Loop() {
				blob, blobErr := gitRepo.BlobObject(blobHash)
				if blobErr != nil {
					b.Fatal(blobErr)
				}
				reader, readErr := blob.Reader()
				if readErr != nil {
					b.Fatal(readErr)
				}
				content := make([]byte, blob.Size)
				if _, readContentErr := reader.Read(content); readContentErr != nil {
					b.Fatal(readContentErr)
				}
				_ = reader.Close()

				_, decompressErr := compression.Decompress(content)
				if decompressErr != nil {
					b.Fatal(decompressErr)
				}
			}
		})
	}
}

// BenchmarkSimulatedPushPayload estimates the total blob size that would be transferred
// during a git push with N compressed checkpoints.
func BenchmarkSimulatedPushPayload(b *testing.B) {
	counts := []int{50, 100, 200}
	transcriptSize := 500 * 1024 // 500KB per transcript (typical session)

	for _, n := range counts {
		b.Run(fmt.Sprintf("%d_checkpoints", n), func(b *testing.B) {
			transcript := generateBenchJSONL(transcriptSize)

			// Measure compressed size
			compressed, err := compression.Compress(transcript)
			if err != nil {
				b.Fatal(err)
			}

			originalTotal := int64(n) * int64(len(transcript))
			compressedTotal := int64(n) * int64(len(compressed))

			b.ReportMetric(float64(originalTotal), "original_bytes")
			b.ReportMetric(float64(compressedTotal), "compressed_bytes")
			b.ReportMetric(float64(originalTotal)/float64(compressedTotal), "ratio")

			// Run actual compression to measure throughput
			b.SetBytes(int64(len(transcript)))
			for b.Loop() {
				_, err := compression.Compress(transcript)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkMigrateCheckpoints simulates migrating uncompressed checkpoints to compressed format.
func BenchmarkMigrateCheckpoints(b *testing.B) {
	transcriptSize := 200 * 1024 // 200KB per transcript
	checkpointCount := 50

	b.Run(fmt.Sprintf("%d_checkpoints", checkpointCount), func(b *testing.B) {
		transcript := generateBenchJSONL(transcriptSize)

		repo := benchutil.NewBenchRepo(b, benchutil.RepoOpts{FileCount: 5, FileSizeLines: 10})
		gitRepo, err := gogit.PlainOpen(repo.Dir)
		if err != nil {
			b.Fatal(err)
		}

		// Create uncompressed blobs (simulating old format)
		var blobHashes []plumbing.Hash
		for range checkpointCount {
			hash, blobErr := checkpoint.CreateBlobFromContent(gitRepo, transcript)
			if blobErr != nil {
				b.Fatal(blobErr)
			}
			blobHashes = append(blobHashes, hash)
		}

		b.SetBytes(int64(checkpointCount) * int64(len(transcript)))
		b.ResetTimer()

		for b.Loop() {
			// Simulate: read each blob, compress, write new blob
			for _, hash := range blobHashes {
				blob, blobErr := gitRepo.BlobObject(hash)
				if blobErr != nil {
					b.Fatal(blobErr)
				}
				reader, readErr := blob.Reader()
				if readErr != nil {
					b.Fatal(readErr)
				}
				content := make([]byte, blob.Size)
				if _, readContentErr := reader.Read(content); readContentErr != nil {
					b.Fatal(readContentErr)
				}
				_ = reader.Close()

				compressed, compressErr := compression.Compress(content)
				if compressErr != nil {
					b.Fatal(compressErr)
				}
				_, blobErr = checkpoint.CreateBlobFromContent(gitRepo, compressed)
				if blobErr != nil {
					b.Fatal(blobErr)
				}
			}
		}
	})
}
