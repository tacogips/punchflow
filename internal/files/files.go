package files

import (
	"context"
	"io"
	"net/http"
	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

type Store struct {
	dir      string
	key      []byte
	baseURL  string
	clock    kftime.Clock
	maxBytes int64
}

func New(dataDir string, key []byte, baseURL string, clock kftime.Clock, maxBytes int64) *Store {
	return &Store{dir: dataDir, key: append([]byte(nil), key...), baseURL: baseURL, clock: clock, maxBytes: maxBytes}
}
func (s *Store) Save(ctx context.Context, q store.Querier, uploaderID, filename, contentType string, r io.Reader) (model.File, error) {
	return model.File{}, kferr.New(500, "internal_server_error", "files not implemented")
}
func (s *Store) SignedURL(f model.File) string { return "" }
func (s *Store) DownloadHandler(db *store.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kferr.Write(w, kferr.New(500, "internal_server_error", "files not implemented"))
	})
}
func (s *Store) MaxBytes() int64 { return s.maxBytes }
