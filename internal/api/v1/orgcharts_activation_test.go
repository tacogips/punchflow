package v1_test

import (
	"net/http"
	"testing"
	"time"

	"punchflow/internal/testutil"
)

const (
	activationCurrentChart = "00000000-0000-4000-8000-000000000027"
	activationNextChart    = "00000000-0000-4000-8000-000000000028"
)

type chartView struct {
	ID             string `json:"id"`
	Current        bool   `json:"current"`
	ActivationPlan *struct {
		ID        string `json:"id"`
		DueOn     string `json:"dueOn"`
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
	} `json:"activationPlan"`
}

func getChart(t *testing.T, env *testutil.Env, id string) chartView {
	t.Helper()
	res := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationCharts/"+id, nil)
	if res.Status != http.StatusOK {
		t.Fatalf("get chart %s: status=%d body=%s", id, res.Status, res.Body)
	}
	var out chartView
	res.JSON(t, &out)
	return out
}

func TestActivateOrganizationChartWithDueOnSchedulesActivation(t *testing.T) {
	env := testutil.New(t)
	res := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+activationNextChart+"/activate", map[string]any{"dueOn": "2026-02-01"})
	if res.Status != http.StatusOK {
		t.Fatalf("schedule status=%d body=%s", res.Status, res.Body)
	}
	var scheduled chartView
	res.JSON(t, &scheduled)
	if scheduled.Current {
		t.Fatal("a future dueOn must not activate the chart immediately")
	}
	if scheduled.ActivationPlan == nil || scheduled.ActivationPlan.DueOn != "2026-02-01" || scheduled.ActivationPlan.ID == "" || scheduled.ActivationPlan.CreatedAt == "" {
		t.Fatalf("activationPlan = %+v, want dueOn 2026-02-01", scheduled.ActivationPlan)
	}
	if current := getChart(t, env, activationCurrentChart); !current.Current || current.ActivationPlan != nil {
		t.Fatalf("current chart changed before the due date: %+v", current)
	}
	if next := getChart(t, env, activationNextChart); next.Current || next.ActivationPlan == nil {
		t.Fatalf("scheduled chart view = %+v", next)
	}

	// The day before the due date nothing changes.
	env.Clock.Set(time.Date(2026, time.January, 31, 14, 0, 0, 0, time.UTC)) // 2026-01-31 23:00 JST
	if current := getChart(t, env, activationCurrentChart); !current.Current {
		t.Fatal("chart switched before the due date")
	}

	// At the start of the due date (JST) the next request applies the plan.
	env.Clock.Set(time.Date(2026, time.January, 31, 15, 0, 0, 0, time.UTC)) // 2026-02-01 00:00 JST
	next := getChart(t, env, activationNextChart)
	if !next.Current || next.ActivationPlan != nil {
		t.Fatalf("due chart after the due date = %+v, want current without a plan", next)
	}
	if old := getChart(t, env, activationCurrentChart); old.Current {
		t.Fatal("previous chart is still current after the scheduled activation")
	}
	current := env.Do(t, testutil.Admin, http.MethodGet, "/v1/organizationChart", nil)
	var currentView chartView
	current.JSON(t, &currentView)
	if currentView.ID != activationNextChart {
		t.Fatalf("current chart = %s, want %s", currentView.ID, activationNextChart)
	}
}

func TestActivateOrganizationChartImmediateForms(t *testing.T) {
	for name, body := range map[string]any{
		"no body":   nil,
		"null":      map[string]any{"dueOn": nil},
		"today JST": map[string]any{"dueOn": "2026-01-01"},
	} {
		t.Run(name, func(t *testing.T) {
			env := testutil.New(t)
			res := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+activationNextChart+"/activate", body)
			if res.Status != http.StatusOK {
				t.Fatalf("activate status=%d body=%s", res.Status, res.Body)
			}
			var out chartView
			res.JSON(t, &out)
			if !out.Current || out.ActivationPlan != nil {
				t.Fatalf("immediate activation = %+v, want current without a plan", out)
			}
		})
	}
}

func TestActivateOrganizationChartValidatesDueOn(t *testing.T) {
	env := testutil.New(t)
	path := "/v1/organizationCharts/" + activationNextChart + "/activate"
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPost, path, map[string]any{"dueOn": "2026/02/01"}), http.StatusBadRequest, "invalid_parameter")
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPost, path, map[string]any{"dueOn": "2025-12-31"}), http.StatusUnprocessableEntity, "validation_failed")
	if chart := getChart(t, env, activationNextChart); chart.Current || chart.ActivationPlan != nil {
		t.Fatalf("rejected requests must not change the chart: %+v", chart)
	}
	// The chart that is already current cannot be scheduled.
	current := "/v1/organizationCharts/" + activationCurrentChart + "/activate"
	testutil.AssertError(t, env.Do(t, testutil.Admin, http.MethodPost, current, map[string]any{"dueOn": "2026-03-01"}), http.StatusUnprocessableEntity, "validation_failed")
}

func TestScheduledActivationIsReplacedAndClearedByImmediateActivation(t *testing.T) {
	env := testutil.New(t)
	path := "/v1/organizationCharts/" + activationNextChart + "/activate"
	if res := env.Do(t, testutil.Admin, http.MethodPost, path, map[string]any{"dueOn": "2026-02-01"}); res.Status != http.StatusOK {
		t.Fatalf("first schedule: %d %s", res.Status, res.Body)
	}
	if res := env.Do(t, testutil.Admin, http.MethodPost, path, map[string]any{"dueOn": "2026-03-01"}); res.Status != http.StatusOK {
		t.Fatalf("reschedule: %d %s", res.Status, res.Body)
	}
	if chart := getChart(t, env, activationNextChart); chart.ActivationPlan == nil || chart.ActivationPlan.DueOn != "2026-03-01" {
		t.Fatalf("rescheduled plan = %+v, want dueOn 2026-03-01", chart.ActivationPlan)
	}
	// Activating the currently active chart immediately discards pending schedules.
	if res := env.Do(t, testutil.Admin, http.MethodPost, "/v1/organizationCharts/"+activationCurrentChart+"/activate", nil); res.Status != http.StatusOK {
		t.Fatalf("immediate activation: %d %s", res.Status, res.Body)
	}
	if chart := getChart(t, env, activationNextChart); chart.ActivationPlan != nil {
		t.Fatalf("immediate activation left a schedule behind: %+v", chart.ActivationPlan)
	}
	env.Clock.Set(time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC))
	if chart := getChart(t, env, activationCurrentChart); !chart.Current {
		t.Fatal("a discarded schedule must not activate its chart later")
	}
}
