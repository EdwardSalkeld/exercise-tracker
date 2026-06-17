package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/EdwardSalkeld/workout-service/internal/model"
)

type fakeStore struct {
	healthErr         error
	workouts          []model.WorkoutSummary
	workout           model.WorkoutDetail
	workoutErr        error
	updatedWorkout    model.WorkoutDetail
	updatedWorkoutErr error
	deletedWorkoutErr error
	createdWorkout    model.WorkoutDetail
	createdWorkoutErr error
	history           []model.ExerciseHistoryItem
	historyErr        error
	runs              []model.RunSummary
	run               model.RunDetail
	runErr            error
	updatedRun        model.RunDetail
	updatedRunErr     error
	deletedRunErr     error
	createdRun        model.RunDetail
	createdRunErr     error
}

func (f fakeStore) HealthCheck(context.Context) error { return f.healthErr }
func (f fakeStore) ListWorkouts(context.Context, int) ([]model.WorkoutSummary, error) {
	return f.workouts, nil
}
func (f fakeStore) GetWorkout(context.Context, int64) (model.WorkoutDetail, error) {
	return f.workout, f.workoutErr
}
func (f fakeStore) UpdateWorkout(context.Context, int64, model.WorkoutCreate) (model.WorkoutDetail, error) {
	return f.updatedWorkout, f.updatedWorkoutErr
}
func (f fakeStore) DeleteWorkout(context.Context, int64) error { return f.deletedWorkoutErr }
func (f fakeStore) ExerciseHistory(context.Context, string, int) ([]model.ExerciseHistoryItem, error) {
	return f.history, f.historyErr
}
func (f fakeStore) ListRuns(context.Context, int) ([]model.RunSummary, error) {
	return f.runs, nil
}
func (f fakeStore) GetRun(context.Context, int64) (model.RunDetail, error) {
	return f.run, f.runErr
}
func (f fakeStore) UpdateRun(context.Context, int64, model.RunCreate) (model.RunDetail, error) {
	return f.updatedRun, f.updatedRunErr
}
func (f fakeStore) DeleteRun(context.Context, int64) error { return f.deletedRunErr }
func (f fakeStore) CreateWorkout(context.Context, model.WorkoutCreate) (model.WorkoutDetail, error) {
	return f.createdWorkout, f.createdWorkoutErr
}
func (f fakeStore) CreateRun(context.Context, model.RunCreate) (model.RunDetail, error) {
	return f.createdRun, f.createdRunErr
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	server := NewServer(fakeStore{})
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestWorkoutNotFound(t *testing.T) {
	t.Parallel()

	server := NewServer(fakeStore{workoutErr: errNotFound})
	request := httptest.NewRequest(http.MethodGet, "/v1/workouts/42", nil)
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestExerciseHistory(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 6, 17, 5, 0, 0, 0, time.UTC)
	server := NewServer(fakeStore{
		history: []model.ExerciseHistoryItem{
			{
				WorkoutID:        7,
				WorkoutTitle:     "Upper",
				WorkoutStartedAt: startedAt,
				DisplayName:      "Lat Pulldown",
				SetNumber:        1,
			},
		},
	})
	request := httptest.NewRequest(http.MethodGet, "/v1/exercises/Lat%20Pulldown/history?limit=10", nil)
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}

	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["exercise"] != "Lat Pulldown" {
		t.Fatalf("exercise = %#v, want %q", payload["exercise"], "Lat Pulldown")
	}
}

func TestRunError(t *testing.T) {
	t.Parallel()

	server := NewServer(fakeStore{runErr: errors.New("boom")})
	request := httptest.NewRequest(http.MethodGet, "/v1/runs/9", nil)
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestCreateWorkout(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 6, 17, 6, 0, 0, 0, time.UTC)
	server := NewServer(fakeStore{
		createdWorkout: model.WorkoutDetail{
			WorkoutSummary: model.WorkoutSummary{
				ID:        11,
				Title:     "Upper",
				StartedAt: startedAt,
			},
			SourceType: "manual",
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/workouts", strings.NewReader(`{
		"title":"Upper",
		"started_at":"2026-06-17T06:00:00Z",
		"source_type":"manual",
		"exercises":[{"display_name":"Lat Pulldown","base_name":"Lat Pulldown","sets":[{"reps":10,"weight_kg":45}]}]
	}`))
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
}

func TestCreateWorkoutBadJSON(t *testing.T) {
	t.Parallel()

	server := NewServer(fakeStore{})
	request := httptest.NewRequest(http.MethodPost, "/v1/workouts", strings.NewReader(`{"title":"Upper"`))
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestUpdateWorkout(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 6, 17, 7, 0, 0, 0, time.UTC)
	server := NewServer(fakeStore{
		updatedWorkout: model.WorkoutDetail{
			WorkoutSummary: model.WorkoutSummary{
				ID:        12,
				Title:     "Upper v2",
				StartedAt: startedAt,
			},
			SourceType: "manual",
		},
	})

	request := httptest.NewRequest(http.MethodPut, "/v1/workouts/11", strings.NewReader(`{
		"title":"Upper v2",
		"started_at":"2026-06-17T07:00:00Z",
		"source_type":"manual",
		"exercises":[{"display_name":"Lat Pulldown","base_name":"Lat Pulldown","sets":[{"reps":8,"weight_kg":50}]}]
	}`))
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestDeleteWorkout(t *testing.T) {
	t.Parallel()

	server := NewServer(fakeStore{})
	request := httptest.NewRequest(http.MethodDelete, "/v1/workouts/11", nil)
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestCreateRun(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 6, 17, 6, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(30 * time.Minute)
	server := NewServer(fakeStore{
		createdRun: model.RunDetail{
			RunSummary: model.RunSummary{
				ID:              9,
				Title:           "Morning Run",
				Sport:           "running",
				StartedAt:       startedAt,
				EndedAt:         endedAt,
				DurationSeconds: 1800,
				DistanceM:       5000,
			},
			SourceType: "manual",
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/runs", strings.NewReader(`{
		"title":"Morning Run",
		"sport":"running",
		"started_at":"2026-06-17T06:00:00Z",
		"ended_at":"2026-06-17T06:30:00Z",
		"duration_seconds":1800,
		"distance_m":5000,
		"source_type":"manual",
		"points":[{"recorded_at":"2026-06-17T06:00:00Z","lat":53.8,"lon":-1.5}]
	}`))
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
}

func TestUpdateRun(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 6, 17, 8, 0, 0, 0, time.UTC)
	endedAt := startedAt.Add(35 * time.Minute)
	server := NewServer(fakeStore{
		updatedRun: model.RunDetail{
			RunSummary: model.RunSummary{
				ID:              10,
				Title:           "Morning Run v2",
				Sport:           "running",
				StartedAt:       startedAt,
				EndedAt:         endedAt,
				DurationSeconds: 2100,
				DistanceM:       5500,
			},
			SourceType: "manual",
		},
	})

	request := httptest.NewRequest(http.MethodPut, "/v1/runs/9", strings.NewReader(`{
		"title":"Morning Run v2",
		"sport":"running",
		"started_at":"2026-06-17T08:00:00Z",
		"ended_at":"2026-06-17T08:35:00Z",
		"duration_seconds":2100,
		"distance_m":5500,
		"source_type":"manual"
	}`))
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestDeleteRun(t *testing.T) {
	t.Parallel()

	server := NewServer(fakeStore{})
	request := httptest.NewRequest(http.MethodDelete, "/v1/runs/9", nil)
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}
