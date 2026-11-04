package store

import (
	"context"
	"fmt"
)

// CalculationVariables returns fixture-backed global formula values by name.
func CalculationVariables(ctx context.Context, q Querier) (map[string]any, error) {
	rows, err := q.QueryContext(ctx, "SELECT name,value FROM calculation_variables ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("store: query calculation variables: %w", err)
	}
	defer func() { _ = rows.Close() }()
	values := map[string]any{}
	for rows.Next() {
		var name string
		var value float64
		if err := rows.Scan(&name, &value); err != nil {
			return nil, fmt.Errorf("store: scan calculation variable: %w", err)
		}
		values[name] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate calculation variables: %w", err)
	}
	return values, nil
}
