package hash

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"

	"github.com/zeebo/blake3"
)

const bufSize = 1 << 20 // 1 MiB

type Algorithm string

const (
	SHA256 Algorithm = "sha256"
	BLAKE3 Algorithm = "blake3"
)

type Hasher struct {
	alg Algorithm
}

var blake3New = func() hash.Hash { return blake3.New() }

// New returns a Hasher using the requested algorithm
// If alg is unknown, it falls back to SHA-256
func New(alg Algorithm) Hasher {
	switch alg {
	case SHA256, BLAKE3:
		return Hasher{alg: alg}
	default:
		return Hasher{alg: SHA256}
	}
}

func (h Hasher) newHash() hash.Hash {
	switch h.alg {
	case BLAKE3:
		return blake3New()
	default:
		return sha256.New()
	}
}

// File computes the content hash of a file at path.
func (h Hasher) File(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("hash: lstat %q: %w", path, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("hash: %q is a directory", path)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if path, err = os.Readlink(path); err != nil {
			return "", fmt.Errorf("hash: readlink %q: %w", path, err)
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("hash: open %q: %w", path, err)
	}
	defer f.Close()

	return h.Reader(f)
}

// Reader hashes arbitrary content from r.
func (h Hasher) Reader(r io.Reader) (string, error) {
	d := h.newHash()
	buf := make([]byte, bufSize)
	if _, err := io.CopyBuffer(d, r, buf); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return "", fmt.Errorf("hash: copy reader: unexpected EOF: %w", err)
		}
		return "", fmt.Errorf("hash: copy reader: %w", err)
	}
	return hex.EncodeToString(d.Sum(nil)), nil
}

// ---- Convenience helpers so callers dont need to wire a Hasher ----

// DefaultAlg is the algorithm used by FileHash/ReaderHash
// Change to SHA256 if you want zero third-party dep by default.
var DefaultAlg = BLAKE3

// FileHash hashes the file at the path using DefaultAlg.
func FileHash(path string) (string, error) {
	return New(DefaultAlg).File(path)
}

// ReaderHash hashes bytes from r using DefaultAlg
func ReaderHash(r io.Reader) (string, error) {
	return New(DefaultAlg).Reader(r)
}
