package hevy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/EdwardSalkeld/exercise-tracker/internal/api"
	"github.com/EdwardSalkeld/exercise-tracker/internal/model"
)

type fakeStore struct {
	syncState        model.SyncState
	syncStateErr     error
	activeWorkoutIDs map[string]int64
	created          []model.WorkoutCreate
	updated          []updateCall
	deleted          []int64
	updateStateVals  []string
	createBaseID     int64
	updateBaseID     int64
	createErr        error
	updateErr        error
	deleteErr        error
	upsertErr        error
}

type updateCall struct {
	id    int64
	input model.WorkoutCreate
}

func (f *fakeStore) GetSyncState(context.Context, string, string) (model.SyncState, error) {
	return f.syncState, f.syncStateErr
}

func (f *fakeStore) UpsertSyncState(_ context.Context, _ string, _ string, value string) (model.SyncState, error) {
	if f.upsertErr != nil {
		return model.SyncState{}, f.upsertErr
	}
	f.updateStateVals = append(f.updateStateVals, value)
	return model.SyncState{Value: value}, nil
}

func (f *fakeStore) CreateWorkout(_ context.Context, input model.WorkoutCreate) (model.WorkoutDetail, error) {
	if f.createErr != nil {
		return model.WorkoutDetail{}, f.createErr
	}
	f.created = append(f.created, input)
	id := f.createBaseID + int64(len(f.created))
	return model.WorkoutDetail{WorkoutSummary: model.WorkoutSummary{ID: id}}, nil
}

func (f *fakeStore) UpdateWorkout(_ context.Context, id int64, input model.WorkoutCreate) (model.WorkoutDetail, error) {
	if f.updateErr != nil {
		return model.WorkoutDetail{}, f.updateErr
	}
	f.updated = append(f.updated, updateCall{id: id, input: input})
	newID := f.updateBaseID + int64(len(f.updated))
	return model.WorkoutDetail{WorkoutSummary: model.WorkoutSummary{ID: newID}}, nil
}

func (f *fakeStore) DeleteWorkout(_ context.Context, id int64) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func (f *fakeStore) ListActiveWorkoutIDsBySourceType(context.Context, string) (map[string]int64, error) {
	result := make(map[string]int64, len(f.activeWorkoutIDs))
	for key, value := range f.activeWorkoutIDs {
		result[key] = value
	}
	return result, nil
}

type fakeClient struct {
	workouts []Workout
	events   []WorkoutEvent
	err      error
}

func (f *fakeClient) IterWorkouts(context.Context, int) ([]Workout, error) {
	return f.workouts, f.err
}

func (f *fakeClient) IterWorkoutEvents(context.Context, string, int) ([]WorkoutEvent, error) {
	return f.events, f.err
}

func TestSyncWorkoutsFallsBackToFullAndDeletesLeftovers(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		syncStateErr:     api.ErrNotFound(),
		activeWorkoutIDs: map[string]int64{"stale": 9, "keep": 11},
		createBaseID:     100,
		updateBaseID:     200,
	}
	client := &fakeClient{
		workouts: []Workout{
			testWorkout("keep", "Existing workout"),
			testWorkout("new", "New workout"),
		},
	}

	result, err := SyncWorkouts(context.Background(), store, client, SyncOptions{
		Now: func() time.Time { return time.Date(2026, 6, 23, 13, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("SyncWorkouts() error = %v", err)
	}

	if result.Mode != "full" {
		t.Fatalf("mode = %q, want full", result.Mode)
	}
	if result.CreatedCount != 1 || result.UpdatedCount != 1 || result.DeletedCount != 1 {
		t.Fatalf("counts = %+v", result)
	}
	if len(store.created) != 1 || len(store.updated) != 1 || len(store.deleted) != 1 {
		t.Fatalf("store ops = created:%d updated:%d deleted:%d", len(store.created), len(store.updated), len(store.deleted))
	}
	if store.deleted[0] != 9 {
		t.Fatalf("deleted id = %d, want 9", store.deleted[0])
	}
	if len(store.updateStateVals) != 1 || store.updateStateVals[0] != "2026-06-23T13:00:00Z" {
		t.Fatalf("sync state updates = %#v", store.updateStateVals)
	}
}

func TestSyncWorkoutsUsesEventsAndCollapsesFinalState(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		syncState:        model.SyncState{Value: "2026-06-23T12:04:52Z"},
		activeWorkoutIDs: map[string]int64{"keep": 22, "gone": 23},
		createBaseID:     100,
		updateBaseID:     200,
	}
	client := &fakeClient{
		events: []WorkoutEvent{
			{Type: "updated", Workout: pointerWorkout(testWorkout("keep", "Keep v1"))},
			{Type: "updated", Workout: pointerWorkout(testWorkout("keep", "Keep v2"))},
			{Type: "updated", Workout: pointerWorkout(testWorkout("new", "Brand new"))},
			{Type: "deleted", ID: "gone"},
			{Type: "deleted", ID: "new"},
		},
	}

	result, err := SyncWorkouts(context.Background(), store, client, SyncOptions{
		Now: func() time.Time { return time.Date(2026, 6, 23, 13, 10, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("SyncWorkouts() error = %v", err)
	}

	if result.Mode != "events" || result.Since != "2026-06-23T12:04:52Z" {
		t.Fatalf("result = %+v", result)
	}
	if result.WorkoutCount != 1 || result.CreatedCount != 0 || result.UpdatedCount != 1 || result.DeletedCount != 1 {
		t.Fatalf("counts = %+v", result)
	}
	if len(store.updated) != 1 || store.updated[0].id != 22 {
		t.Fatalf("updated = %#v", store.updated)
	}
	if got := store.updated[0].input.Title; got != "Keep v2" {
		t.Fatalf("updated title = %q, want %q", got, "Keep v2")
	}
	if len(store.deleted) != 1 || store.deleted[0] != 23 {
		t.Fatalf("deleted = %#v", store.deleted)
	}
}

func TestSyncWorkoutsDoesNotAdvanceStateOnWriteFailure(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		syncState:        model.SyncState{Value: "2026-06-23T12:04:52Z"},
		activeWorkoutIDs: map[string]int64{"keep": 22},
		updateErr:        errors.New("boom"),
	}
	client := &fakeClient{
		events: []WorkoutEvent{
			{Type: "updated", Workout: pointerWorkout(testWorkout("keep", "Keep v2"))},
		},
	}

	_, err := SyncWorkouts(context.Background(), store, client, SyncOptions{})
	if err == nil {
		t.Fatal("SyncWorkouts() error = nil, want failure")
	}
	if len(store.updateStateVals) != 0 {
		t.Fatalf("sync state updates = %#v, want none", store.updateStateVals)
	}
}

func testWorkout(id string, title string) Workout {
	return Workout{
		ID:          id,
		Title:       title,
		StartTime:   "2026-06-23T12:00:00Z",
		EndTime:     "2026-06-23T13:00:00Z",
		Description: "notes",
		Exercises: []WorkoutExercise{
			{
				Index: 0,
				Title: "Bench Press",
				Sets: []ExerciseSet{
					{
						Index:    0,
						WeightKG: float64Ptr(60),
						Reps:     float64Ptr(8),
					},
				},
			},
		},
	}
}

func pointerWorkout(workout Workout) *Workout {
	return &workout
}

func float64Ptr(value float64) *float64 {
	return &value
}
