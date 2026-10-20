package cli

import (
	"context"
	"fmt"
	"strings"

	"punchflow/internal/auth"
	"punchflow/internal/kftime"
	"punchflow/internal/model"
	"punchflow/internal/store"
)

func runToken(ctx context.Context, args []string, stdout, stderr interface{ Write([]byte) (int, error) }) error {
	if len(args) == 0 || args[0] != "issue" {
		return usageError{err: fmt.Errorf("token requires issue")}
	}
	fs := cliFlagSet("token issue", stderr)
	common := commonFlags(fs)
	user := fs.String("user", "", "user ID, code, or e-mail")
	service := fs.String("service-account", "", "service account name")
	name := fs.String("name", "cli", "token name")
	if err := parseFlags(fs, args[1:]); err != nil {
		return err
	}
	if (*user == "") == (*service == "") {
		return usageError{err: fmt.Errorf("exactly one of --user or --service-account is required")}
	}
	common.resolve()
	db, err := store.Open(ctx, *common.dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	authn := auth.NewAuthenticator(db, kftime.SystemClock{})
	kind, userID := model.TokenKindPAT, ""
	if *service != "" {
		kind = model.TokenKindSAT
		*name = strings.TrimSpace(*service)
	} else {
		if strings.Contains(*user, "@") {
			u, err := store.UserByEmail(ctx, db.Q(), *user)
			if err != nil {
				return fmt.Errorf("resolve user: %w", err)
			}
			userID = u.ID
		} else {
			userID, err = store.ResolveID(ctx, db.Q(), "users", *user)
			if err != nil {
				return fmt.Errorf("resolve user: %w", err)
			}
		}
	}
	token, _, err := authn.IssueToken(ctx, db.Q(), kind, userID, *name, "")
	if err != nil {
		return fmt.Errorf("issue token: %w", err)
	}
	_, err = fmt.Fprintln(stdout, token)
	return err
}
