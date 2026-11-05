package httpclient

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	responseDirectoryName = "responses"
	staleResponseAge      = 24 * time.Hour
	defaultChunkSize      = 64 << 10
)

type storedResponse struct {
	path        string
	contentType string
	encoding    string
	size        int64
}

// store keeps response bodies in files under the user cache directory. Files
// are named after opaque identifiers, so a response is only reachable through
// the identifier returned to the frontend.
type store struct {
	mutex     sync.Mutex
	directory string
	responses map[string]storedResponse
}

func newStore() (*store, error) {
	cacheDirectory, err := os.UserCacheDir()
	if err != nil {
		cacheDirectory = os.TempDir()
	}

	return newStoreAt(filepath.Join(cacheDirectory, "awake", responseDirectoryName))
}

func newStoreAt(directory string) (*store, error) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("create response directory: %w", err)
	}

	removeStaleResponses(directory)

	return &store{
		directory: directory,
		responses: make(map[string]storedResponse),
	}, nil
}

func (responseStore *store) temporaryFile() (*os.File, error) {
	return os.CreateTemp(responseStore.directory, "response-*")
}

func (responseStore *store) register(path string, contentType string, encoding string, size int64) (string, error) {
	identifier, err := randomIdentifier()
	if err != nil {
		return "", err
	}

	responseStore.mutex.Lock()
	defer responseStore.mutex.Unlock()
	responseStore.responses[identifier] = storedResponse{
		path:        path,
		contentType: contentType,
		encoding:    encoding,
		size:        size,
	}

	return identifier, nil
}

func (responseStore *store) readChunk(identifier string, offset int64, length int) (Chunk, error) {
	responseStore.mutex.Lock()
	response, found := responseStore.responses[identifier]
	responseStore.mutex.Unlock()

	if !found {
		return Chunk{}, ErrResponseNotFound
	}
	if offset < 0 || offset > response.size {
		return Chunk{}, ErrInvalidChunk
	}
	if length <= 0 {
		length = defaultChunkSize
	}
	if length > MaximumChunkSize {
		length = MaximumChunkSize
	}

	remaining := response.size - offset
	if remaining <= 0 {
		return Chunk{Encoding: response.encoding, NextOffset: offset}, nil
	}
	if int64(length) > remaining {
		length = int(remaining)
	}

	file, err := os.Open(response.path)
	if err != nil {
		return Chunk{}, fmt.Errorf("open response %s: %w", identifier, err)
	}
	defer file.Close()

	buffer := make([]byte, length)
	read, err := file.ReadAt(buffer, offset)
	if err != nil && err != io.EOF {
		return Chunk{}, fmt.Errorf("read response %s: %w", identifier, err)
	}
	buffer = buffer[:read]

	encoding := response.encoding
	if encoding == "utf-8" && !utf8.Valid(buffer) {
		encoding = "base64"
	}

	nextOffset := offset + int64(read)

	return Chunk{
		Content:    encodeBytes(buffer, encoding),
		Encoding:   encoding,
		NextOffset: nextOffset,
		HasMore:    nextOffset < response.size,
	}, nil
}

func (responseStore *store) delete(identifier string) error {
	responseStore.mutex.Lock()
	response, found := responseStore.responses[identifier]
	delete(responseStore.responses, identifier)
	responseStore.mutex.Unlock()

	if !found {
		return ErrResponseNotFound
	}

	if err := os.Remove(response.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove response %s: %w", identifier, err)
	}

	return nil
}

func encodeBytes(content []byte, encoding string) string {
	if encoding == "base64" {
		return base64.StdEncoding.EncodeToString(content)
	}
	return string(content)
}

func randomIdentifier() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate response identifier: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func removeStaleResponses(directory string) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}

	cutoff := time.Now().Add(-staleResponseAge)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(directory, entry.Name()))
	}
}
