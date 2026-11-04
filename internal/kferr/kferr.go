// Package kferr defines kickflow-compatible API errors.
package kferr

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
)

// Error is an HTTP error in kickflow's API error format.
type Error struct {
	Status  int                 `json:"-"`
	Code    string              `json:"code"`
	Message string              `json:"message"`
	Errors  map[string][]string `json:"errors,omitempty"`
}

// Error returns the human-readable error message.
func (e *Error) Error() string { return e.Message }

// New constructs an API error.
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// As finds an API error wrapped by err.
func As(err error) (*Error, bool) {
	var apiError *Error
	if errors.As(err, &apiError) {
		return apiError, true
	}
	return nil, false
}

// Write writes err as a compact JSON response. Unknown errors become HTTP 500.
func Write(w http.ResponseWriter, err error) {
	apiError, ok := As(err)
	if !ok || apiError == nil {
		apiError = Internal()
	}
	response := struct {
		Code    string               `json:"code"`
		Message string               `json:"message"`
		Errors  *map[string][]string `json:"errors,omitempty"`
	}{Code: apiError.Code, Message: apiError.Message}
	if apiError.Errors != nil {
		response.Errors = &apiError.Errors
	}
	body, marshalErr := json.Marshal(response)
	if marshalErr != nil {
		apiError = Internal()
		response = struct {
			Code    string               `json:"code"`
			Message string               `json:"message"`
			Errors  *map[string][]string `json:"errors,omitempty"`
		}{Code: apiError.Code, Message: apiError.Message}
		body, _ = json.Marshal(response)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(apiError.Status)
	_, _ = w.Write(body)
}

func coded(status int, code, message string) *Error { return New(status, code, message) }

func badRequest(code, message string) *Error   { return coded(http.StatusBadRequest, code, message) }
func unauthorized(code, message string) *Error { return coded(http.StatusUnauthorized, code, message) }
func forbidden(code, message string) *Error    { return coded(http.StatusForbidden, code, message) }
func notFound(code, message string) *Error     { return coded(http.StatusNotFound, code, message) }
func unprocessable(code, message string) *Error {
	return coded(http.StatusUnprocessableEntity, code, message)
}

func InvalidParameter(msg string) *Error  { return badRequest("invalid_parameter", msg) }
func InvalidContentType() *Error          { return badRequest("invalid_content_type", "Invalid content type") }
func InvalidJSON() *Error                 { return badRequest("invalid_json", "Invalid JSON") }
func PayloadTooLarge() *Error             { return badRequest("payload_too_large", "Payload too large") }
func InvalidHeader(msg string) *Error     { return badRequest("invalid_header", msg) }
func InvalidAccessToken() *Error          { return unauthorized("invalid_access_token", "Invalid access token") }
func UserNotRegistered() *Error           { return unauthorized("user_not_registered", "User is not registered") }
func UserInactive() *Error                { return unauthorized("user_inactive", "User is inactive") }
func InvalidCallerID() *Error             { return unauthorized("invalid_caller_id", "Invalid caller ID") }
func InvitationExpired() *Error           { return unauthorized("invitation_expired", "Invitation expired") }
func MissingPermission(msg string) *Error { return forbidden("missing_permission", msg) }
func UpgradeRequired() *Error             { return forbidden("upgrade_required", "Upgrade required") }
func InvisibleTicket() *Error             { return forbidden("invisible_ticket", "Ticket is not visible") }
func UserNotAssigned() *Error             { return forbidden("user_not_assigned", "User is not assigned") }
func TicketCreateRestricted() *Error {
	return forbidden("ticket_create_restricted", "Ticket creation is restricted")
}
func IPAddressDenied() *Error { return forbidden("ip_address_denied", "IP address denied") }
func AdminIsWorking() *Error  { return forbidden("admin_is_working", "Administrator is working") }
func TenantSuspended() *Error { return forbidden("tenant_suspended", "Tenant is suspended") }
func TenantMaintenance() *Error {
	return forbidden("tenant_maintenance", "Tenant is under maintenance")
}
func ResourceNotEditable() *Error {
	return forbidden("resource_not_editable", "Resource is not editable")
}
func ResourceNotFound() *Error { return notFound("resource_not_found", "Resource not found") }
func EndpointNotFound() *Error { return notFound("endpoint_not_found", "Endpoint not found") }

// ValidationFailed creates a 422 error and deterministically summarizes the first error per field.
func ValidationFailed(fieldErrors map[string][]string) *Error {
	fields := make([]string, 0, len(fieldErrors))
	for field := range fieldErrors {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	messages := make([]string, 0, len(fields))
	for _, field := range fields {
		if len(fieldErrors[field]) > 0 {
			messages = append(messages, field+" "+fieldErrors[field][0])
		}
	}
	message := strings.Join(messages, "; ")
	return &Error{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: message, Errors: fieldErrors}
}

func InvalidState(msg string) *Error { return unprocessable("invalid_state", msg) }
func DomainNotAllowed() *Error {
	return unprocessable("domain_not_allowed", "Email domain is not allowed")
}
func AssignmentRequiredBeforeApprove() *Error {
	return unprocessable("assignment_required_before_approve", "Assignment is required before approval")
}
func InputRequiredBeforeApprove() *Error {
	return unprocessable("input_required_before_approve", "Input is required before approval")
}
func RateLimited() *Error {
	return coded(http.StatusTooManyRequests, "rate_limited", "Rate limit exceeded")
}
func Internal() *Error {
	return coded(http.StatusInternalServerError, "internal_server_error", "Internal server error")
}
