package auth

import (
	"database/sql"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"punchflow/internal/kferr"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// Authenticator resolves API tokens and browser sessions against the store.
type Authenticator struct {
	db    *store.DB
	clock kftime.Clock
}

// NewAuthenticator constructs an authenticator using the supplied clock.
func NewAuthenticator(db *store.DB, clock kftime.Clock) *Authenticator {
	if clock == nil {
		clock = kftime.SystemClock{}
	}
	return &Authenticator{db: db, clock: clock}
}

// Middleware authenticates the request, applies account and tenant restrictions, then calls next.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		now := a.clock.Now()
		q := a.db.Q()
		var user model.User
		var principal Principal
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
				kferr.Write(w, kferr.InvalidAccessToken())
				return
			}
			token, err := store.TokenByHash(ctx, q, HashToken(parts[1]))
			if err != nil {
				if err == store.ErrNotFound || err == sql.ErrNoRows {
					kferr.Write(w, kferr.InvalidAccessToken())
				} else {
					kferr.Write(w, err)
				}
				return
			}
			if err = store.TouchToken(ctx, q, token.ID, now); err != nil {
				kferr.Write(w, err)
				return
			}
			principal.Via = token.Kind
			principal.TokenID = token.ID
			userID := ""
			if token.Kind == model.TokenKindSAT {
				caller := r.Header.Get("X-Caller-Id")
				if caller == "" {
					kferr.Write(w, kferr.InvalidCallerID())
					return
				}
				if !store.IsUUID(caller) {
					kferr.Write(w, kferr.InvalidHeader("Invalid X-Caller-Id"))
					return
				}
				userID = caller
			} else if token.UserID != nil {
				userID = *token.UserID
			} else {
				kferr.Write(w, kferr.InvalidAccessToken())
				return
			}
			user, err = store.UserByID(ctx, q, userID)
			if err != nil {
				if token.Kind == model.TokenKindSAT && (err == store.ErrNotFound || err == sql.ErrNoRows) {
					kferr.Write(w, kferr.InvalidCallerID())
				} else if err == store.ErrNotFound || err == sql.ErrNoRows {
					kferr.Write(w, kferr.InvalidAccessToken())
				} else {
					kferr.Write(w, err)
				}
				return
			}
		} else {
			cookie, err := r.Cookie(SessionCookie)
			if err != nil {
				kferr.Write(w, kferr.InvalidAccessToken())
				return
			}
			session, err := store.SessionByHash(ctx, q, HashToken(cookie.Value))
			if err != nil {
				if err == store.ErrNotFound || err == sql.ErrNoRows {
					kferr.Write(w, kferr.InvalidAccessToken())
				} else {
					kferr.Write(w, err)
				}
				return
			}
			if pf05IsSessionExpired(session, now) {
				if err = store.DeleteSession(ctx, q, session.IDHash); err != nil {
					kferr.Write(w, err)
					return
				}
				kferr.Write(w, kferr.InvalidAccessToken())
				return
			}
			if r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
				if r.Header.Get(CSRFHeader) != session.CSRFToken {
					kferr.Write(w, kferr.MissingPermission("csrf token mismatch"))
					return
				}
			}
			user, err = store.UserByID(ctx, q, session.UserID)
			if err != nil {
				kferr.Write(w, kferr.InvalidAccessToken())
				return
			}
			if err = store.TouchSession(ctx, q, session.IDHash, now); err != nil {
				kferr.Write(w, err)
				return
			}
			principal.Via = "session"
			principal.CSRFToken = session.CSRFToken
		}
		if user.Status == model.UserStatusInvited {
			invitation, err := store.InvitationByUser(ctx, q, user.ID)
			if err != nil || !now.Before(invitation.ExpiresAt) {
				kferr.Write(w, kferr.InvitationExpired())
			} else {
				kferr.Write(w, kferr.UserNotRegistered())
			}
			return
		}
		if user.Status == model.UserStatusSuspended || user.Status == model.UserStatusDeactivated {
			kferr.Write(w, kferr.UserInactive())
			return
		}
		tenant, err := store.Tenant(ctx, q)
		if err != nil {
			kferr.Write(w, err)
			return
		}
		switch tenant.Mode {
		case model.TenantModeSuspended:
			kferr.Write(w, kferr.TenantSuspended())
			return
		case model.TenantModeMaintenance:
			kferr.Write(w, kferr.TenantMaintenance())
			return
		}
		if len(tenant.IPAllowlist) > 0 && !pf05IPAllowed(tenant.IPAllowlist, r.RemoteAddr) {
			kferr.Write(w, kferr.IPAddressDenied())
			return
		}
		if user.LastUsedOn == nil || *user.LastUsedOn != kftime.FormatDate(now) {
			if err = store.TouchLastUsedOn(ctx, q, user.ID, kftime.FormatDate(now)); err != nil {
				kferr.Write(w, err)
				return
			}
		}
		// Lazily apply a scheduled organization chart activation that has come due.
		if _, err = store.ApplyDueOrgChartActivation(ctx, a.db, kftime.FormatDate(now), now); err != nil {
			kferr.Write(w, err)
			return
		}
		// Future-dated proxy registrations can become effective without a
		// ticket transition. Materialize their sharing before authorization and
		// reads so the first request on the start date sees the new access.
		if err = store.ApplyEffectiveProxySharing(ctx, a.db, kftime.FormatDate(now), now); err != nil {
			kferr.Write(w, err)
			return
		}
		grants, err := store.UserGrants(ctx, q, user.ID)
		if err != nil {
			kferr.Write(w, err)
			return
		}
		permissions := make(map[string]bool, len(grants))
		for permission := range grants {
			permissions[permission] = true
		}
		principal.User = user
		principal.Permissions = permissions
		principal.Grants = grants
		next.ServeHTTP(w, r.WithContext(WithPrincipal(ctx, &principal)))
	})
}

func pf05IPAllowed(entries []string, remote string) bool {
	host := remote
	if parsed, _, err := net.SplitHostPort(remote); err == nil {
		host = parsed
	}
	addr, err := netip.ParseAddr(strings.Trim(host, "[]"))
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if prefix, err := netip.ParsePrefix(entry); err == nil {
			if prefix.Contains(addr) {
				return true
			}
			continue
		}
		if ip, err := netip.ParseAddr(entry); err == nil && ip.Unmap() == addr {
			return true
		}
	}
	return false
}
