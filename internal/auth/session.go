package auth

import (
	"context"
	"fmt"
	"time"

	"punchflow/internal/kferr"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

// ActivateOnLogin activates an invited user only while the stored invitation is valid.
func (a *Authenticator) ActivateOnLogin(ctx context.Context, q store.Querier, user model.User) (model.User, error) {
	now := a.clock.Now()
	switch user.Status {
	case model.UserStatusActive:
		return user, nil
	case model.UserStatusInvited:
		invitation, err := store.InvitationByUser(ctx, q, user.ID)
		if err != nil {
			return model.User{}, kferr.InvitationExpired()
		}
		if !now.Before(invitation.ExpiresAt) {
			return model.User{}, kferr.InvitationExpired()
		}
		if err = store.ActivateUser(ctx, q, user.ID, now); err != nil {
			return model.User{}, fmt.Errorf("auth: activate invited user: %w", err)
		}
		user.Status = model.UserStatusActive
		return user, nil
	default:
		return model.User{}, kferr.UserInactive()
	}
}

func pf05IsSessionExpired(s model.Session, now time.Time) bool {
	return now.Sub(s.LastSeenAt) > 12*time.Hour || now.Sub(s.CreatedAt) > 24*time.Hour
}
