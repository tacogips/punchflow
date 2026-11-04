package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"punchflow/internal/model"
)

// RootFolderID returns the ID of the tenant's root folder: the non-editable
// folder without a parent (falling back to any parentless folder).
func RootFolderID(ctx context.Context, q Querier) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, "SELECT id FROM folders WHERE parent_id IS NULL ORDER BY editable ASC, created_at ASC, id ASC LIMIT 1").Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("store: read root folder: %w", err)
	}
	return id, nil
}

// FolderTreeDefinitionFolders returns the distinct folders within folderIDs that
// directly contain live (not deleted) workflows and routes respectively.
func FolderTreeDefinitionFolders(ctx context.Context, q Querier, folderIDs []string) (workflowFolders, routeFolders []string, err error) {
	if len(folderIDs) == 0 {
		return nil, nil, nil
	}
	marks, args := inList(folderIDs)
	workflowFolders, err = distinctFolders(ctx, q, "workflows", marks, args)
	if err != nil {
		return nil, nil, err
	}
	routeFolders, err = distinctFolders(ctx, q, "routes", marks, args)
	if err != nil {
		return nil, nil, err
	}
	return workflowFolders, routeFolders, nil
}

func distinctFolders(ctx context.Context, q Querier, table, marks string, args []any) ([]string, error) {
	rows, err := q.QueryContext(ctx, "SELECT DISTINCT folder_id FROM "+table+" WHERE status!='deleted' AND folder_id IN ("+marks+") ORDER BY folder_id", args...)
	if err != nil {
		return nil, fmt.Errorf("store: list %s folders: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan %s folder: %w", table, err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DeleteFolderTree deletes the folder and every descendant folder. Workflows and
// routes inside the tree are retired (status deleted) rather than removed so
// existing tickets keep their workflow and route history; retired definitions
// are re-homed to the root folder because a definition must always have a
// folder. The caller must run it inside a transaction.
func DeleteFolderTree(ctx context.Context, q Querier, id string, at time.Time) (deleted []string, err error) {
	tree, err := FolderSubtreeIDs(ctx, q, []string{id})
	if err != nil {
		return nil, err
	}
	if len(tree) == 0 {
		return nil, ErrNotFound
	}
	root, err := RootFolderID(ctx, q)
	if err != nil {
		return nil, err
	}
	marks, args := inList(tree)
	for _, table := range []string{"workflows", "routes"} {
		updateArgs := append([]any{model.WorkflowStatusDeleted, root, Millis(at)}, args...)
		if _, err = q.ExecContext(ctx, "UPDATE "+table+" SET status=?, folder_id=?, updated_at=? WHERE folder_id IN ("+marks+")", updateArgs...); err != nil {
			return nil, fmt.Errorf("store: retire %s of deleted folders: %w", table, err)
		}
	}
	if _, err = q.ExecContext(ctx, "DELETE FROM folders WHERE id IN ("+marks+")", args...); err != nil {
		return nil, fmt.Errorf("store: delete folder tree: %w", err)
	}
	return tree, nil
}

func inList(ids []string) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(ids)), ","), args
}
