package store

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type ObjectStore struct {
	basePath string
}

func New(basePath string) *ObjectStore {
	return &ObjectStore{basePath: filepath.Join(basePath, "objects")}
}

func (s *ObjectStore) Init() error {
	return os.MkdirAll(s.basePath, 0755)
}

func Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (s *ObjectStore) Has(hash string) bool {
	_, err := os.Stat(s.objectPath(hash))
	return err == nil
}

func (s *ObjectStore) Write(data []byte) (string, error) {
	hash := Hash(data)

	if s.Has(hash) {
		return hash, nil
	}

	dir := filepath.Join(s.basePath, hash[:2])
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create object dir: %w", err)
	}

	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return "", fmt.Errorf("compress: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("close compressor: %w", err)
	}

	path := s.objectPath(hash)
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("write object: %w", err)
	}

	return hash, nil
}

func (s *ObjectStore) Read(hash string) ([]byte, error) {
	path := s.objectPath(hash)

	compressed, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read object %s: %w", hash[:8], err)
	}

	r, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("decompress %s: %w", hash[:8], err)
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read decompressed %s: %w", hash[:8], err)
	}

	return data, nil
}

func (s *ObjectStore) ReadRaw(hash string) ([]byte, error) {
	path := s.objectPath(hash)
	return os.ReadFile(path)
}

func (s *ObjectStore) WriteRaw(hash string, data []byte) error {
	dir := filepath.Join(s.basePath, hash[:2])
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create object dir: %w", err)
	}
	path := s.objectPath(hash)
	return os.WriteFile(path, data, 0644)
}

func (s *ObjectStore) objectPath(hash string) string {
	return filepath.Join(s.basePath, hash[:2], hash[2:])
}
