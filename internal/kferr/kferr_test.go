package kferr

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteValidationFailed(t *testing.T) {
	response := httptest.NewRecorder()
	Write(response, ValidationFailed(map[string][]string{"comment": {"must not be empty"}}))
	want := `{"code":"validation_failed","message":"comment must not be empty","errors":{"comment":["must not be empty"]}}`
	if response.Code != 422 || response.Body.String() != want {
		t.Fatalf("Write() = status %d body %q, want status 422 body %q", response.Code, response.Body.String(), want)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestWriteIncludesNonNilEmptyValidationErrors(t *testing.T) {
	response := httptest.NewRecorder()
	Write(response, ValidationFailed(map[string][]string{}))
	want := `{"code":"validation_failed","message":"","errors":{}}`
	if response.Code != 422 || response.Body.String() != want {
		t.Fatalf("Write() = status %d body %q, want status 422 body %q", response.Code, response.Body.String(), want)
	}
}

func TestWriteUnknownErrorAndHTMLMessage(t *testing.T) {
	response := httptest.NewRecorder()
	Write(response, errors.New("boom"))
	if response.Code != 500 || !strings.Contains(response.Body.String(), `"code":"internal_server_error"`) {
		t.Fatalf("unknown error response = status %d body %q", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	Write(response, InvalidParameter("bad <a&b>"))
	if !strings.Contains(response.Body.String(), `bad \u003ca\u0026b\u003e`) {
		t.Fatalf("HTML-sensitive text was not escaped: %q", response.Body.String())
	}
	if strings.HasSuffix(response.Body.String(), "\n") {
		t.Fatalf("response has trailing newline: %q", response.Body.String())
	}
}

func TestAsFindsWrappedError(t *testing.T) {
	want := InvalidParameter("bad")
	got, ok := As(fmt.Errorf("wrapped: %w", want))
	if !ok || got != want {
		t.Fatalf("As() = (%v, %v), want (%v, true)", got, ok, want)
	}
}

func TestDocumentedConstructors(t *testing.T) {
	constructors := []struct {
		name   string
		call   func() *Error
		code   string
		status int
	}{
		{"InvalidParameter", func() *Error { return InvalidParameter("x") }, "invalid_parameter", 400},
		{"InvalidContentType", InvalidContentType, "invalid_content_type", 400},
		{"InvalidJSON", InvalidJSON, "invalid_json", 400},
		{"PayloadTooLarge", PayloadTooLarge, "payload_too_large", 400},
		{"InvalidHeader", func() *Error { return InvalidHeader("x") }, "invalid_header", 400},
		{"InvalidAccessToken", InvalidAccessToken, "invalid_access_token", 401},
		{"UserNotRegistered", UserNotRegistered, "user_not_registered", 401},
		{"UserInactive", UserInactive, "user_inactive", 401},
		{"InvalidCallerID", InvalidCallerID, "invalid_caller_id", 401},
		{"InvitationExpired", InvitationExpired, "invitation_expired", 401},
		{"MissingPermission", func() *Error { return MissingPermission("x") }, "missing_permission", 403},
		{"UpgradeRequired", UpgradeRequired, "upgrade_required", 403},
		{"InvisibleTicket", InvisibleTicket, "invisible_ticket", 403},
		{"UserNotAssigned", UserNotAssigned, "user_not_assigned", 403},
		{"TicketCreateRestricted", TicketCreateRestricted, "ticket_create_restricted", 403},
		{"IPAddressDenied", IPAddressDenied, "ip_address_denied", 403},
		{"AdminIsWorking", AdminIsWorking, "admin_is_working", 403},
		{"TenantSuspended", TenantSuspended, "tenant_suspended", 403},
		{"TenantMaintenance", TenantMaintenance, "tenant_maintenance", 403},
		{"ResourceNotEditable", ResourceNotEditable, "resource_not_editable", 403},
		{"ResourceNotFound", ResourceNotFound, "resource_not_found", 404},
		{"EndpointNotFound", EndpointNotFound, "endpoint_not_found", 404},
		{"InvalidState", func() *Error { return InvalidState("x") }, "invalid_state", 422},
		{"DomainNotAllowed", DomainNotAllowed, "domain_not_allowed", 422},
		{"AssignmentRequiredBeforeApprove", AssignmentRequiredBeforeApprove, "assignment_required_before_approve", 422},
		{"InputRequiredBeforeApprove", InputRequiredBeforeApprove, "input_required_before_approve", 422},
		{"RateLimited", RateLimited, "rate_limited", 429},
		{"Internal", Internal, "internal_server_error", 500},
	}
	for _, tc := range constructors {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.call()
			if got.Code != tc.code || got.Status != tc.status {
				t.Fatalf("constructor = (%d, %q), want (%d, %q)", got.Status, got.Code, tc.status, tc.code)
			}
		})
	}
}
