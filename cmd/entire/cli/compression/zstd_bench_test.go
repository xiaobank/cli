package compression

import (
	"fmt"
	"strings"
	"testing"
)

// generateRealisticJSONL creates JSONL data that resembles Claude Code transcripts.
func generateRealisticJSONL(targetSize int) []byte {
	var buf strings.Builder
	lineTemplates := []string{
		`{"type":"human","message":{"id":"msg_%06d","content":"Please implement the feature for handling user authentication with JWT tokens and session management"}}`,
		`{"type":"assistant","message":{"id":"msg_%06d","content":"I'll implement the authentication feature. Let me start by creating the middleware...","model":"claude-sonnet-4-5-20250514","stop_reason":"end_turn","usage":{"input_tokens":1234,"output_tokens":5678}}}`,
		`{"type":"tool_use","id":"toolu_%06d","name":"Write","input":{"file_path":"/src/auth/middleware.go","content":"package auth\n\nfunc Middleware() {}"},"output":"File written successfully"}`,
		`{"type":"tool_result","id":"toolu_%06d","content":"Successfully wrote 45 bytes to /src/auth/middleware.go"}`,
	}

	i := 0
	for buf.Len() < targetSize {
		template := lineTemplates[i%len(lineTemplates)]
		line := fmt.Sprintf(template, i)
		buf.WriteString(line)
		buf.WriteByte('\n')
		i++
	}

	return []byte(buf.String())
}

func BenchmarkCompress(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
		{"25MB", 25 * 1024 * 1024},
	}

	for _, s := range sizes {
		data := generateRealisticJSONL(s.size)
		b.Run(s.name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			var compressedSize int
			for b.Loop() {
				compressed, err := Compress(data)
				if err != nil {
					b.Fatal(err)
				}
				compressedSize = len(compressed)
			}
			ratio := float64(len(data)) / float64(compressedSize)
			b.ReportMetric(ratio, "ratio")
			b.ReportMetric(float64(compressedSize), "compressed_bytes")
		})
	}
}

func BenchmarkDecompress(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
		{"25MB", 25 * 1024 * 1024},
	}

	for _, s := range sizes {
		data := generateRealisticJSONL(s.size)
		compressed, err := Compress(data)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(s.name, func(b *testing.B) {
			b.SetBytes(int64(len(data)))
			for b.Loop() {
				_, err := Decompress(compressed)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
