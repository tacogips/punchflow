package testutil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnvPersonasAndCallerFor(t *testing.T) {
	env := New(t)
	for _, persona := range []Persona{Admin, Manager, Finance, Employee, Integration} {
		t.Run(string(persona), func(t *testing.T) {
			response := env.Do(t, persona, http.MethodGet, "/v1/user", nil)
			if response.Status == http.StatusUnauthorized {
				t.Fatalf("persona %s was not authenticated: %s", persona, response.Body)
			}
		})
	}

	unauthenticated := env.Do(t, "", http.MethodGet, "/v1/user", nil)
	AssertError(t, unauthenticated, http.StatusUnauthorized, "invalid_access_token")

	env.CallerFor = "finance-manager"
	integration := env.Do(t, Integration, http.MethodGet, "/v1/user", nil)
	if integration.Status == http.StatusUnauthorized {
		t.Fatalf("integration persona with CallerFor=%q was not authenticated: %s", env.CallerFor, integration.Body)
	}
	if got, want := env.UserID(t, "admin"), "00000000-0000-4000-8000-000000000007"; got != want {
		t.Fatalf("admin user ID = %q, want %q", got, want)
	}
}

func TestReceiverRecordsCopiesAndUsesResponseSequence(t *testing.T) {
	receiver := newReceiver()
	t.Cleanup(receiver.server.Close)
	receiver.RespondWith(http.StatusServiceUnavailable, http.StatusAccepted)

	for i := 0; i < 3; i++ {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, receiver.server.URL, strings.NewReader("payload"))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-Test", "value")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		wantStatus := []int{http.StatusServiceUnavailable, http.StatusAccepted, http.StatusAccepted}[i]
		if response.StatusCode != wantStatus {
			t.Fatalf("response %d status = %d, want %d", i, response.StatusCode, wantStatus)
		}
	}

	requests := receiver.Requests()
	if len(requests) != 3 {
		t.Fatalf("received %d requests, want 3", len(requests))
	}
	requests[0].Body[0] = 'X'
	requests[0].Header.Set("X-Test", "changed")
	again := receiver.Requests()
	if got, want := string(again[0].Body), "payload"; got != want {
		t.Fatalf("stored body changed through returned snapshot: %q, want %q", got, want)
	}
	if got, want := again[0].Header.Get("X-Test"), "value"; got != want {
		t.Fatalf("stored header changed through returned snapshot: %q, want %q", got, want)
	}
}

func TestTestutilAssertions(t *testing.T) {
	response := &Response{Status: http.StatusBadRequest, Header: http.Header{
		"Page":     []string{"2"},
		"Per-Page": []string{"25"},
		"Total":    []string{"30"},
		"Link":     []string{`</v1/users?page=1>; rel="first"`},
	}, Body: []byte(`{"code":"invalid_parameter","message":"bad request"}`)}
	AssertError(t, response, http.StatusBadRequest, "invalid_parameter")
	AssertPagination(t, response, 2, 25, 30)
	var decoded map[string]any
	response.JSON(t, &decoded)
	AssertKeys(t, decoded, "code", "message")

	encoded, err := json.Marshal(decoded)
	if err != nil || !strings.Contains(string(encoded), `"invalid_parameter"`) {
		t.Fatalf("JSON round trip = %q, error = %v", encoded, err)
	}
}

func TestDoRawResolvesRelativeURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("path = %q, want /healthz", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	env := &Env{URL: server.URL}
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := env.DoRaw(t, request)
	if response.Status != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Status)
	}
}
