package v1_test

import (
	"bytes"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"testing"

	"punchflow/internal/testutil"
)

func TestUploadAndGetFileAttachment(t *testing.T) {
	env := testutil.New(t)
	data := []byte("receipt bytes")
	response := pf23UploadFile(t, env, data, true)
	if response.Status != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", response.Status, response.Body)
	}
	var uploaded struct {
		SignedID string `json:"signedId"`
	}
	response.JSON(t, &uploaded)
	if uploaded.SignedID == "" {
		t.Fatal("upload returned an empty signedId")
	}
	unattached := pf23Request(t, env, http.MethodGet, "/v1/files/"+uploaded.SignedID, nil, "")
	if unattached.Status != http.StatusNotFound {
		t.Fatalf("unattached file status=%d body=%s", unattached.Status, unattached.Body)
	}
	pf23CreateExpenseTicket(t, env, "sales-rep", "sales", uploaded.SignedID)
	metadata := pf23Request(t, env, http.MethodGet, "/v1/files/"+uploaded.SignedID, nil, "")
	if metadata.Status != http.StatusOK {
		t.Fatalf("file metadata status=%d body=%s", metadata.Status, metadata.Body)
	}
	var file struct {
		URL         string `json:"url"`
		Filename    string `json:"filename"`
		Checksum    string `json:"checksum"`
		ByteSize    int64  `json:"byteSize"`
		ContentType string `json:"contentType"`
		CreatedAt   string `json:"createdAt"`
	}
	metadata.JSON(t, &file)
	if file.URL == "" || file.Filename != "receipt.pdf" || file.ByteSize != int64(len(data)) || file.ContentType != "application/pdf" || file.CreatedAt == "" || file.Checksum == "" {
		t.Fatalf("unexpected metadata: %+v", file)
	}
	download := pf23Request(t, env, http.MethodGet, file.URL, nil, "")
	if download.Status != http.StatusOK || !bytes.Equal(download.Body, data) {
		t.Fatalf("download status/body=%d/%q", download.Status, download.Body)
	}
	if !strings.Contains(download.Header.Get("Content-Disposition"), "filename*=UTF-8''receipt.pdf") {
		t.Fatalf("Content-Disposition=%q", download.Header.Get("Content-Disposition"))
	}
}

func TestUploadRejectsInvalidFormAndStandardPlan(t *testing.T) {
	env := testutil.New(t)
	jsonResponse := pf23Request(t, env, http.MethodPost, "/v1/files", []byte(`{"file":"x"}`), "application/json")
	if jsonResponse.Status != http.StatusBadRequest {
		t.Fatalf("JSON upload status=%d body=%s", jsonResponse.Status, jsonResponse.Body)
	}
	missing := pf23UploadFile(t, env, nil, false)
	if missing.Status != http.StatusBadRequest {
		t.Fatalf("missing part status=%d body=%s", missing.Status, missing.Body)
	}
	if _, err := env.DB.Q().ExecContext(t.Context(), `UPDATE tenant SET plan='standard'`); err != nil {
		t.Fatal(err)
	}
	blocked := pf23UploadFile(t, env, []byte("x"), true)
	if blocked.Status != http.StatusForbidden {
		t.Fatalf("standard plan status=%d body=%s", blocked.Status, blocked.Body)
	}
}

func pf23UploadFile(t *testing.T, env *testutil.Env, data []byte, includeFile bool) *testutil.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if includeFile {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": "receipt.pdf"}))
		header.Set("Content-Type", "application/pdf")
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = part.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return pf23Request(t, env, http.MethodPost, "/v1/files", body.Bytes(), writer.FormDataContentType())
}

func pf23Request(t *testing.T, env *testutil.Env, method, path string, body []byte, contentType string) *testutil.Response {
	t.Helper()
	requestURL := path
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		requestURL = env.URL + path
	}
	request, err := http.NewRequestWithContext(t.Context(), method, requestURL, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer pf_pat_dev_employee")
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	return env.DoRaw(t, request)
}
