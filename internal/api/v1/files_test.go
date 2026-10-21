package v1_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"testing"

	"punchflow/internal/model"
	"punchflow/internal/store"
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

func TestRestrictedSectionFileRequiresSectionAccessForMetadataAndDownload(t *testing.T) {
	env := testutil.New(t)
	workflow, err := store.GetWorkflowByCode(t.Context(), env.DB.Q(), "expense-claim")
	if err != nil {
		t.Fatal(err)
	}
	definition, err := store.WorkflowDefByID(t.Context(), env.DB.Q(), workflow.ID)
	if err != nil {
		t.Fatal(err)
	}
	var fileFieldID string
	var fileSectionID string
	for i := range definition.SectionList {
		for _, field := range definition.SectionList[i].FormFields {
			if field.FieldType == "file" {
				fileFieldID = field.ID
				fileSectionID = definition.SectionList[i].ID
				definition.SectionList[i].ViewRestriction = true
				definition.SectionList[i].AllowViewAuthor = true
				definition.SectionList[i].AllowViewApprover = false
				definition.SectionList[i].SectionViewers = []model.SectionViewer{{ID: "restricted-file-viewer", UserCode: "ceo"}}
			}
		}
	}
	if fileFieldID == "" {
		t.Fatal("fixture has no file field")
	}
	ticketID := pf23CreateExpenseTicket(t, env, "sales-rep", "sales", "")
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE workflows SET definition_json=? WHERE id=?`, string(encoded), workflow.ID); err != nil {
		t.Fatal(err)
	}
	uploaded := pf23UploadFile(t, env, []byte("restricted receipt"), true)
	if uploaded.Status != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", uploaded.Status, uploaded.Body)
	}
	var uploadBody struct {
		SignedID string `json:"signedId"`
	}
	uploaded.JSON(t, &uploadBody)
	file, err := store.FileBySignedID(t.Context(), env.DB.Q(), uploadBody.SignedID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `INSERT INTO file_attachments(file_id,ticket_id,form_field_id,created_at) VALUES(?,?,?,0)`, file.ID, ticketID, fileFieldID); err != nil {
		t.Fatal(err)
	}
	metadata := env.Do(t, testutil.Employee, http.MethodGet, "/v1/files/"+uploadBody.SignedID, nil)
	if metadata.Status != http.StatusOK {
		t.Fatalf("authorized metadata status=%d body=%s", metadata.Status, metadata.Body)
	}
	var response struct {
		URL string `json:"url"`
	}
	metadata.JSON(t, &response)
	if response.URL == "" {
		t.Fatal("authorized response has no download URL")
	}
	downloadSignedURL := func() *testutil.Response {
		req, requestErr := http.NewRequestWithContext(context.Background(), http.MethodGet, response.URL, nil)
		if requestErr != nil {
			t.Fatal(requestErr)
		}
		return env.DoRaw(t, req)
	}
	if got := downloadSignedURL(); got.Status != http.StatusOK || !bytes.Equal(got.Body, []byte("restricted receipt")) {
		t.Fatalf("authorized download status/body=%d/%q", got.Status, got.Body)
	}
	for i := range definition.SectionList {
		if definition.SectionList[i].ID == fileSectionID {
			definition.SectionList[i].AllowViewAuthor = false
		}
	}
	encoded, err = json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.DB.Q().ExecContext(t.Context(), `UPDATE workflows SET definition_json=? WHERE id=?`, string(encoded), workflow.ID); err != nil {
		t.Fatal(err)
	}
	denied := env.Do(t, testutil.Employee, http.MethodGet, "/v1/files/"+uploadBody.SignedID, nil)
	testutil.AssertError(t, denied, http.StatusForbidden, "invisible_ticket")
	if got := downloadSignedURL(); got.Status != http.StatusForbidden {
		t.Fatalf("download after section access revoked status=%d body=%s, want forbidden", got.Status, got.Body)
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

func TestUploadUnsupportedExtensionIsInvalidParameter(t *testing.T) {
	env := testutil.New(t)
	response := pf23UploadNamedFile(t, env, "example.exe", []byte("x"))
	testutil.AssertError(t, response, http.StatusBadRequest, "invalid_parameter")
}

func pf23UploadFile(t *testing.T, env *testutil.Env, data []byte, includeFile bool) *testutil.Response {
	return pf23UploadNamedFile(t, env, "receipt.pdf", data, includeFile)
}

func pf23UploadNamedFile(t *testing.T, env *testutil.Env, filename string, data []byte, includeFile ...bool) *testutil.Response {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	withFile := len(includeFile) == 0 || includeFile[0]
	if withFile {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": "file", "filename": filename}))
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
