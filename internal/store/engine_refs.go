package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"punchflow/internal/model"
)

func EngUserExists(ctx context.Context, q Querier, id string) (bool, error) {
	_, err := GetUser(ctx, q, id)
	if err == ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: check engine user: %w", err)
	}
	return true, nil
}

func EngTeamInActiveChart(ctx context.Context, q Querier, id string) (bool, error) {
	team, err := GetTeam(ctx, q, id)
	if err == ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: load engine team: %w", err)
	}
	chartID, err := ActiveOrgChartID(ctx, q)
	if err != nil {
		return false, fmt.Errorf("store: load active engine chart: %w", err)
	}
	return team.OrganizationChartID == chartID, nil
}

func EngTicketStatus(ctx context.Context, q Querier, id string) (string, bool, error) {
	ticket, err := Ticket(ctx, q, id)
	if err == ErrNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("store: load engine ticket status: %w", err)
	}
	return ticket.Status, true, nil
}

func EngMasterItemInMaster(ctx context.Context, q Querier, itemID, masterCode string) (bool, error) {
	var masterID string
	err := q.QueryRowContext(ctx, `SELECT id FROM general_masters WHERE code=?`, masterCode).Scan(&masterID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("store: find engine master: %w", err)
	}
	item, err := GetGeneralMasterItem(ctx, q, itemID)
	if err == ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: load engine master item: %w", err)
	}
	return item.GeneralMasterID == masterID, nil
}

func EngActiveProxyApplicant(ctx context.Context, q Querier, clientUserID, proxyUserID, workflowID, date string) (bool, error) {
	rows, err := q.QueryContext(ctx, `SELECT starts_on,ends_on,workflow_ids FROM proxy_applicants WHERE user_id=? AND proxy_user_id=?`, clientUserID, proxyUserID)
	if err != nil {
		return false, fmt.Errorf("store: query engine proxy applicants: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var starts, ends *string
		var workflows string
		if err = rows.Scan(&starts, &ends, &workflows); err != nil {
			return false, fmt.Errorf("store: scan engine proxy applicant: %w", err)
		}
		if starts != nil && *starts > date || ends != nil && *ends < date {
			continue
		}
		var allowed []string
		if err = json.Unmarshal([]byte(workflows), &allowed); err != nil {
			return false, fmt.Errorf("store: decode engine proxy applicant workflows: %w", err)
		}
		for _, id := range allowed {
			if id == workflowID {
				return true, nil
			}
		}
	}
	if err = rows.Err(); err != nil {
		return false, fmt.Errorf("store: iterate engine proxy applicants: %w", err)
	}
	return false, nil
}

func EngUserActive(ctx context.Context, q Querier, id string) (bool, error) {
	user, err := GetUser(ctx, q, id)
	if err == ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("store: load engine client user: %w", err)
	}
	return user.Status == model.UserStatusActive && user.DeactivatedAt == nil, nil
}
