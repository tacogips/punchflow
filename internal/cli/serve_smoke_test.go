package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServeSmoke(t *testing.T) {
	tmp := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, writer := io.Pipe()
	done := make(chan int, 1)
	go func() {
		done <- Run(ctx, []string{"serve", "--listen", "127.0.0.1:0", "--db", filepath.Join(tmp, "pf.sqlite"), "--data-dir", tmp}, io.Discard, writer)
		_ = writer.Close()
	}()
	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "listening on http://") {
				ready <- strings.TrimPrefix(line, "listening on ")
				return
			}
		}
		ready <- ""
	}()
	var base string
	select {
	case base = <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not become ready within 10 seconds")
	}
	if base == "" {
		t.Fatal("serve exited before readiness")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	request := func(method, path, token string, body io.Reader) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, base+path, body)
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return resp
	}
	resp := request(http.MethodGet, "/healthz", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	resp = request(http.MethodGet, "/v1/user", "pf_pat_dev_admin", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("current user status = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	ticketBody := `{"status":"in_progress","workflowId":"1f46223b-a6ef-4ee8-884e-d3f1e6b9f13b","authorTeamId":"00000000-0000-4000-8000-000000000021","title":"PF-26 serve smoke","inputs":[{"formFieldId":"6f32e53a-d4ce-4d19-8723-754749c7d803","value":5000},{"formFieldId":"36b5a79c-d51f-45ad-8fab-24fcc9948789","value":"Smoke test"},{"formFieldId":"d10e07d0-0f87-465e-8d5f-c81b9fff9beb","value":"2026-01-01"},{"formFieldId":"6eaffa72-f1cd-4a9f-bb17-4dec8b5449f9","generalMasterItemId":"00000000-0000-4000-8000-000000000041"},{"formFieldId":"12547c53-e398-4b25-b9e2-d90230ba1abe","files":[]},{"formFieldId":"95427a69-4c2c-472e-8799-7653cf1f36cc","value":""}]}`
	resp = request(http.MethodPost, "/v1/tickets", "pf_pat_dev_employee", strings.NewReader(ticketBody))
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		t.Fatalf("employee ticket creation status = %d body=%s", resp.StatusCode, data)
	}
	var ticket struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ticket); err != nil {
		t.Fatalf("decode created ticket: %v", err)
	}
	_ = resp.Body.Close()
	if ticket.Status != "in_progress" {
		t.Fatalf("created ticket status=%q, want in_progress", ticket.Status)
	}
	resp = request(http.MethodGet, "/", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("root status = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("serve exit code = %d", code)
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("serve did not stop; db=%s", filepath.Join(tmp, "pf.sqlite"))
	}
	_ = reader.Close()
}
