package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

const SessionCookie = "punchflow_session"
const CSRFHeader = "X-Punchflow-CSRF"

type RoutesOptions struct {
	EnableLocalLogin bool
	SecureCookie     bool
	OIDC             *OIDC
}

type pf05SessionUser struct {
	ID            string         `json:"id"`
	Code          string         `json:"code"`
	Email         string         `json:"email"`
	FirstName     string         `json:"firstName"`
	LastName      string         `json:"lastName"`
	EmployeeID    *string        `json:"employeeId"`
	Status        string         `json:"status"`
	Locale        string         `json:"locale"`
	UserType      string         `json:"userType"`
	CustomFields  map[string]any `json:"customFields"`
	LastUsedOn    *string        `json:"lastUsedOn"`
	DeactivatedAt *time.Time     `json:"deactivatedAt"`
	CreatedAt     string         `json:"createdAt"`
	UpdatedAt     string         `json:"updatedAt"`
}
type pf05SessionResponse struct {
	User        pf05SessionUser `json:"user"`
	Permissions []string        `json:"permissions"`
	CSRFToken   string          `json:"csrfToken"`
	AuthMethods struct {
		OIDC  bool `json:"oidc"`
		Local bool `json:"local"`
	} `json:"authMethods"`
}

// Register installs browser authentication handlers and OIDC delegate routes.
func (a *Authenticator) Register(mux *http.ServeMux, opt RoutesOptions) {
	mux.HandleFunc("GET /auth/session", func(w http.ResponseWriter, r *http.Request) { a.sessionHandler(w, r, opt) })
	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) { a.logoutHandler(w, r, opt) })
	mux.HandleFunc("POST /auth/local/login", func(w http.ResponseWriter, r *http.Request) { a.localLoginHandler(w, r, opt) })
	mux.HandleFunc("GET /auth/oidc/login", func(w http.ResponseWriter, r *http.Request) {
		if opt.OIDC == nil {
			kferr.Write(w, kferr.EndpointNotFound())
			return
		}
		opt.OIDC.Login(w, r)
	})
	mux.HandleFunc("GET /auth/oidc/callback", func(w http.ResponseWriter, r *http.Request) {
		if opt.OIDC == nil {
			kferr.Write(w, kferr.EndpointNotFound())
			return
		}
		opt.OIDC.Callback(w, r)
	})
}

func (a *Authenticator) sessionHandler(w http.ResponseWriter, r *http.Request, opt RoutesOptions) {
	cookie, err := r.Cookie(SessionCookie)
	if err != nil {
		kferr.Write(w, kferr.InvalidAccessToken())
		return
	}
	session, err := store.SessionByHash(r.Context(), a.db.Q(), HashToken(cookie.Value))
	if err != nil || pf05IsSessionExpired(session, a.clock.Now()) {
		kferr.Write(w, kferr.InvalidAccessToken())
		return
	}
	user, err := store.UserByID(r.Context(), a.db.Q(), session.UserID)
	if err != nil {
		kferr.Write(w, kferr.InvalidAccessToken())
		return
	}
	perms, err := store.UserPermissions(r.Context(), a.db.Q(), user.ID)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	writeSessionResponse(w, user, perms, session.CSRFToken, opt)
}

func (a *Authenticator) logoutHandler(w http.ResponseWriter, r *http.Request, opt RoutesOptions) {
	if cookie, err := r.Cookie(SessionCookie); err == nil {
		if err = store.DeleteSession(r.Context(), a.db.Q(), HashToken(cookie.Value)); err != nil {
			kferr.Write(w, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: opt.SecureCookie, Expires: time.Unix(1, 0), MaxAge: -1})
	w.WriteHeader(http.StatusOK)
}

func (a *Authenticator) localLoginHandler(w http.ResponseWriter, r *http.Request, opt RoutesOptions) {
	if !opt.EnableLocalLogin {
		kferr.Write(w, kferr.EndpointNotFound())
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || strings.TrimSpace(input.Email) == "" {
		kferr.Write(w, kferr.InvalidJSON())
		return
	}
	user, err := store.UserByEmail(r.Context(), a.db.Q(), strings.TrimSpace(input.Email))
	if err != nil {
		if err == store.ErrNotFound {
			kferr.Write(w, kferr.InvalidAccessToken())
		} else {
			kferr.Write(w, err)
		}
		return
	}
	user, err = a.ActivateOnLogin(r.Context(), a.db.Q(), user)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	cookie, csrf, err := a.CreateSession(r.Context(), a.db.Q(), user.ID)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookie, Value: cookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: opt.SecureCookie})
	perms, err := store.UserPermissions(r.Context(), a.db.Q(), user.ID)
	if err != nil {
		kferr.Write(w, err)
		return
	}
	writeSessionResponse(w, user, perms, csrf, opt)
}

func writeSessionResponse(w http.ResponseWriter, user model.User, perms map[string]bool, csrf string, opt RoutesOptions) {
	keys := make([]string, 0, len(perms))
	for key, enabled := range perms {
		if enabled {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	response := pf05SessionResponse{User: pf05SessionUser{ID: user.ID, Code: user.Code, Email: user.Email, FirstName: user.FirstName, LastName: user.LastName, EmployeeID: user.EmployeeID, Status: user.Status, Locale: user.Locale, UserType: user.UserType, CustomFields: user.CustomFields, LastUsedOn: user.LastUsedOn, DeactivatedAt: user.DeactivatedAt, CreatedAt: kftime.Format(user.CreatedAt), UpdatedAt: kftime.Format(user.UpdatedAt)}, Permissions: keys, CSRFToken: csrf}
	response.AuthMethods.OIDC = opt.OIDC != nil
	response.AuthMethods.Local = opt.EnableLocalLogin
	body, err := json.Marshal(response)
	if err != nil {
		kferr.Write(w, kferr.Internal())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
