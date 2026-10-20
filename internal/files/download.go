package files

import (
	"crypto/hmac"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"punchflow/internal/kferr"
	"punchflow/internal/store"
)

func (s *Store) pf23ServeDownload(w http.ResponseWriter, r *http.Request, db *store.DB) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/files/download/")
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		kferr.Write(w, kferr.New(http.StatusForbidden, "invalid_signature", "Invalid or expired download URL"))
		return
	}
	parts := strings.Split(string(decoded), ".")
	if len(parts) != 3 || parts[0] == "" {
		kferr.Write(w, kferr.New(http.StatusForbidden, "invalid_signature", "Invalid or expired download URL"))
		return
	}
	expires, err := strconv.ParseInt(parts[1], 10, 64)
	providedMAC, macErr := hex.DecodeString(parts[2])
	value := parts[0] + "." + parts[1]
	if err != nil || macErr != nil || !hmac.Equal(providedMAC, hmacSHA256(s.key, value)) {
		kferr.Write(w, kferr.New(http.StatusForbidden, "invalid_signature", "Invalid or expired download URL"))
		return
	}
	if expires <= s.clock.Now().Unix() {
		kferr.Write(w, kferr.New(http.StatusForbidden, "expired_signature", "Invalid or expired download URL"))
		return
	}
	file, err := store.FileByID(r.Context(), db.Q(), parts[0])
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	if file.StorageName == "" || filepath.Base(file.StorageName) != file.StorageName || strings.ContainsAny(file.StorageName, `/\\`) {
		kferr.Write(w, kferr.Internal())
		return
	}
	content, err := os.Open(filepath.Join(s.dir, file.StorageName))
	if err != nil {
		kferr.Write(w, kferr.ResourceNotFound())
		return
	}
	defer func() { _ = content.Close() }()
	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(file.Filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", file.ByteSize))
	http.ServeContent(w, r, file.Filename, file.CreatedAt, content)
}
