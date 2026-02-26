package compression

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestCompress_RoundTrip(t *testing.T) {
	t.Parallel()
	input := []byte("hello world, this is a test of zstd compression")
	compressed, err := Compress(input)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}
	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}
	if !bytes.Equal(input, decompressed) {
		t.Fatalf("round-trip mismatch: got %q, want %q", decompressed, input)
	}
}

func TestCompress_EmptyData(t *testing.T) {
	t.Parallel()
	compressed, err := Compress([]byte{})
	if err != nil {
		t.Fatalf("Compress failed on empty data: %v", err)
	}
	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress failed on empty data: %v", err)
	}
	if len(decompressed) != 0 {
		t.Fatalf("expected empty result, got %d bytes", len(decompressed))
	}
}

func TestCompress_LargeJSONL(t *testing.T) {
	t.Parallel()
	// Simulate a large JSONL transcript with repetitive structure
	var buf strings.Builder
	for i := range 1000 {
		buf.WriteString(`{"type":"assistant","content":"This is message `)
		buf.WriteString(strings.Repeat("x", 100))
		buf.WriteString(`","index":`)
		buf.WriteString(strings.Repeat("0", 5))
		if i < 999 {
			buf.WriteString("}\n")
		} else {
			buf.WriteString("}")
		}
	}
	input := []byte(buf.String())

	compressed, err := Compress(input)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	// Verify compression actually reduced size for repetitive JSONL
	if len(compressed) >= len(input) {
		t.Fatalf("expected compression to reduce size: compressed=%d, original=%d", len(compressed), len(input))
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress failed: %v", err)
	}
	if !bytes.Equal(input, decompressed) {
		t.Fatal("round-trip mismatch for large JSONL")
	}
}

func TestCompress_Concurrent(t *testing.T) {
	t.Parallel()
	input := []byte("concurrent test data that should compress and decompress correctly")

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			compressed, err := Compress(input)
			if err != nil {
				t.Errorf("Compress failed: %v", err)
				return
			}
			decompressed, err := Decompress(compressed)
			if err != nil {
				t.Errorf("Decompress failed: %v", err)
				return
			}
			if !bytes.Equal(input, decompressed) {
				t.Errorf("round-trip mismatch in concurrent test")
			}
		}()
	}
	wg.Wait()
}

func TestDecompress_InvalidData(t *testing.T) {
	t.Parallel()
	_, err := Decompress([]byte("this is not valid zstd data"))
	if err == nil {
		t.Fatal("expected error for invalid zstd data")
	}
}

func TestIsCompressedName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want bool
	}{
		{"full.jsonl.zst", true},
		{"full.jsonl.zst.001", false},
		{"full.jsonl", false},
		{"file.zst", true},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsCompressedName(tt.name); got != tt.want {
			t.Errorf("IsCompressedName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCompressedName(t *testing.T) {
	t.Parallel()
	if got := CompressedName("full.jsonl"); got != "full.jsonl.zst" {
		t.Errorf("CompressedName(full.jsonl) = %q, want %q", got, "full.jsonl.zst")
	}
}
