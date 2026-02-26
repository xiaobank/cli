package compression

import (
	"fmt"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// Compress compresses data using zstd with default speed settings.
func Compress(data []byte) ([]byte, error) {
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd encoder: %w", err)
	}
	defer encoder.Close()
	return encoder.EncodeAll(data, make([]byte, 0, len(data)/4)), nil
}

// Decompress decompresses zstd-compressed data.
func Decompress(data []byte) ([]byte, error) {
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
	}
	defer decoder.Close()
	result, err := decoder.DecodeAll(data, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress zstd data: %w", err)
	}
	return result, nil
}

// IsCompressedName returns true if the filename has a .zst suffix.
func IsCompressedName(name string) bool {
	return strings.HasSuffix(name, ".zst")
}

// CompressedName returns the filename with a .zst suffix appended.
func CompressedName(name string) string {
	return name + ".zst"
}
