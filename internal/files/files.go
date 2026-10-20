package files

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/present"
	"punchflow/internal/store"
)

const pf23DefaultMaxBytes int64 = 50 << 20

type Store struct {
	dir      string
	key      []byte
	baseURL  string
	clock    kftime.Clock
	maxBytes int64
}

func New(dataDir string, key []byte, baseURL string, clock kftime.Clock, maxBytes int64) *Store {
	if clock == nil {
		clock = kftime.SystemClock{}
	}
	if maxBytes <= 0 {
		maxBytes = pf23DefaultMaxBytes
	}
	s := &Store{dir: dataDir, key: append([]byte(nil), key...), baseURL: strings.TrimRight(baseURL, "/"), clock: clock, maxBytes: maxBytes}
	present.AttachmentURL = s.SignedURL
	return s
}
func (s *Store) Save(ctx context.Context, q store.Querier, uploaderID, filename, contentType string, r io.Reader) (model.File, error) {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return model.File{}, fmt.Errorf("files: create storage directory: %w", err)
	}
	body, err := io.ReadAll(io.LimitReader(r, s.maxBytes+1))
	if err != nil {
		return model.File{}, fmt.Errorf("files: read upload: %w", err)
	}
	if int64(len(body)) > s.maxBytes {
		return model.File{}, fmt.Errorf("files: upload exceeds maximum size of %d bytes", s.maxBytes)
	}
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(body)
	}
	fileID := store.NewID()
	signed := make([]byte, 32)
	if _, err := rand.Read(signed); err != nil {
		return model.File{}, fmt.Errorf("files: generate signed id: %w", err)
	}
	storageName := strings.ReplaceAll(fileID, "-", "")
	path := filepath.Join(s.dir, storageName)
	if err = os.WriteFile(path, body, 0600); err != nil {
		return model.File{}, fmt.Errorf("files: write upload: %w", err)
	}
	uploader := uploaderID
	checksum := md5.Sum(body)
	file := model.File{
		ID: fileID, SignedID: base64.RawURLEncoding.EncodeToString(signed), Filename: filepath.Base(filename),
		ContentType: contentType, ByteSize: int64(len(body)), Checksum: base64.StdEncoding.EncodeToString(checksum[:]),
		StorageName: storageName, UploadedBy: &uploader, CreatedAt: s.clock.Now(),
	}
	if err = store.FileInsert(ctx, q, file); err != nil {
		_ = os.Remove(path)
		return model.File{}, fmt.Errorf("files: persist upload metadata: %w", err)
	}
	return file, nil
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func (s *Store) SignedURL(f model.File) string {
	expires := s.clock.Now().Add(5 * time.Minute).Unix()
	value := fmt.Sprintf("%s.%d", f.ID, expires)
	mac := hmacSHA256(s.key, value)
	token := base64.RawURLEncoding.EncodeToString([]byte(value + "." + hex.EncodeToString(mac)))
	return s.baseURL + "/files/download/" + token
}
func (s *Store) DownloadHandler(db *store.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { s.pf23ServeDownload(w, r, db) })
}
func (s *Store) MaxBytes() int64 { return s.maxBytes }
