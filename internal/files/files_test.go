package files

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"punchflow/internal/kftime"
	"punchflow/internal/store"
)

func TestSaveSignedURLAndDownload(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "files.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Q().ExecContext(ctx, `INSERT INTO users(id,code,email,first_name,last_name,status,locale,user_type,custom_fields,created_at,updated_at) VALUES('uploader','uploader','uploader@example.test','Upload','User','active','en','employee','{}',0,0)`); err != nil {
		t.Fatal(err)
	}
	clock := kftime.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	root := filepath.Join(t.TempDir(), "uploads")
	fileStore := New(root, []byte("test-signing-key"), "", clock, 1024)
	want := []byte("signed file contents")
	file, err := fileStore.Save(ctx, db.Q(), "uploader", "receipt ü.pdf", "application/pdf", strings.NewReader(string(want)))
	if err != nil {
		t.Fatal(err)
	}
	checksum := md5.Sum(want)
	if file.Checksum != base64.StdEncoding.EncodeToString(checksum[:]) || file.ByteSize != int64(len(want)) {
		t.Fatalf("file checksum/size = %q/%d", file.Checksum, file.ByteSize)
	}
	if file.SignedID == "" || file.StorageName != strings.ReplaceAll(file.ID, "-", "") {
		t.Fatalf("unexpected stored identifiers: %+v", file)
	}
	if _, err := os.Stat(filepath.Join(root, file.StorageName)); err != nil {
		t.Fatalf("generated storage file missing: %v", err)
	}
	handler := fileStore.DownloadHandler(db)
	url := fileStore.SignedURL(file)
	serve := func(path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		return recorder
	}
	good := serve(url)
	if good.Code != http.StatusOK {
		t.Fatalf("download status=%d body=%q", good.Code, good.Body.String())
	}
	body, err := io.ReadAll(good.Body)
	if err != nil || string(body) != string(want) {
		t.Fatalf("download body=%q err=%v", body, err)
	}
	if got := good.Header().Get("Content-Disposition"); !strings.Contains(got, "filename*=UTF-8''receipt%20%C3%BC.pdf") {
		t.Fatalf("Content-Disposition=%q", got)
	}
	if got := serve(url + "x"); got.Code != http.StatusForbidden {
		t.Fatalf("tampered URL status=%d body=%q", got.Code, got.Body.String())
	}
	clock.Advance(6 * time.Minute)
	if got := serve(url); got.Code != http.StatusForbidden {
		t.Fatalf("expired URL status=%d body=%q", got.Code, got.Body.String())
	}
}

func TestSaveRejectsOversize(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	fileStore := New(t.TempDir(), []byte("key"), "", kftime.SystemClock{}, 3)
	if _, err := fileStore.Save(ctx, db.Q(), "uploader", "large.bin", "", strings.NewReader("four")); err == nil {
		t.Fatal("oversize upload succeeded")
	}
}
