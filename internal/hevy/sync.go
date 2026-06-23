package hevy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/EdwardSalkeld/exercise-tracker/internal/api"
	"github.com/EdwardSalkeld/exercise-tracker/internal/model"
)

const (
	SyncStateNamespace = "hevy"
	SyncStateKey       = "workouts_last_event_sync_completed_at"
	SourceType         = "hevy_api"
	DefaultPageSize    = 10
)

var APIKeyEnvVars = []string{
	"EXERCISE_TRACKER_HEVY_API_KEY",
	"WORKOUT_DATA_HEVY_API_KEY",
	"HEVY_API_KEY",
}

type SyncStore interface {
	GetSyncState(ctx context.Context, namespace string, key string) (model.SyncState, error)
	UpsertSyncState(ctx context.Context, namespace string, key string, value string) (model.SyncState, error)
	CreateWorkout(ctx context.Context, input model.WorkoutCreate) (model.WorkoutDetail, error)
	UpdateWorkout(ctx context.Context, id int64, input model.WorkoutCreate) (model.WorkoutDetail, error)
	DeleteWorkout(ctx context.Context, id int64) error
	ListActiveWorkoutIDsBySourceType(ctx context.Context, sourceType string) (map[string]int64, error)
}

type WorkoutClient interface {
	IterWorkouts(ctx context.Context, pageSize int) ([]Workout, error)
	IterWorkoutEvents(ctx context.Context, since string, pageSize int) ([]WorkoutEvent, error)
}

type SyncOptions struct {
	Full     bool
	PageSize int
	Since    string
	Now      func() time.Time
}

type SyncResult struct {
	Mode         string `json:"mode"`
	Since        string `json:"since,omitempty"`
	CompletedAt  string `json:"completed_at"`
	WorkoutCount int    `json:"workout_count"`
	CreatedCount int    `json:"created_count"`
	UpdatedCount int    `json:"updated_count"`
	DeletedCount int    `json:"deleted_count"`
}

func ResolveAPIKey(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit), nil
	}
	for _, envVar := range APIKeyEnvVars {
		if value := strings.TrimSpace(os.Getenv(envVar)); value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("missing Hevy API key; set one of %s", strings.Join(APIKeyEnvVars, ", "))
}

func SyncWorkouts(ctx context.Context, store SyncStore, client WorkoutClient, options SyncOptions) (SyncResult, error) {
	pageSize := options.PageSize
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	nowFn := options.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	if options.Full {
		return syncFull(ctx, store, client, pageSize, nowFn)
	}

	since := strings.TrimSpace(options.Since)
	if since == "" {
		state, err := store.GetSyncState(ctx, SyncStateNamespace, SyncStateKey)
		if err != nil {
			if !errors.Is(err, api.ErrNotFound()) {
				return SyncResult{}, fmt.Errorf("load Hevy sync state: %w", err)
			}
		} else {
			since = strings.TrimSpace(state.Value)
		}
	}
	if since == "" {
		return syncFull(ctx, store, client, pageSize, nowFn)
	}
	return syncEvents(ctx, store, client, since, pageSize, nowFn)
}

func syncFull(ctx context.Context, store SyncStore, client WorkoutClient, pageSize int, nowFn func() time.Time) (SyncResult, error) {
	workouts, err := client.IterWorkouts(ctx, pageSize)
	if err != nil {
		return SyncResult{}, err
	}
	remoteIndex, err := store.ListActiveWorkoutIDsBySourceType(ctx, SourceType)
	if err != nil {
		return SyncResult{}, fmt.Errorf("list active Hevy workouts: %w", err)
	}

	seen := make(map[string]struct{}, len(workouts))
	createdCount := 0
	updatedCount := 0
	for _, workout := range workouts {
		payload, err := buildWorkoutCreate(workout)
		if err != nil {
			return SyncResult{}, err
		}
		externalID := strings.TrimSpace(workout.ID)
		seen[externalID] = struct{}{}
		trackerID, exists := remoteIndex[externalID]
		if !exists {
			created, err := store.CreateWorkout(ctx, payload)
			if err != nil {
				return SyncResult{}, fmt.Errorf("create Hevy workout %s: %w", externalID, err)
			}
			remoteIndex[externalID] = created.ID
			createdCount++
			continue
		}
		updated, err := store.UpdateWorkout(ctx, trackerID, payload)
		if err != nil {
			return SyncResult{}, fmt.Errorf("update Hevy workout %s: %w", externalID, err)
		}
		remoteIndex[externalID] = updated.ID
		updatedCount++
	}

	deletedCount := 0
	for _, externalID := range sortedExternalIDs(remoteIndex) {
		if _, ok := seen[externalID]; ok {
			continue
		}
		if err := store.DeleteWorkout(ctx, remoteIndex[externalID]); err != nil {
			return SyncResult{}, fmt.Errorf("delete stale Hevy workout %s: %w", externalID, err)
		}
		deletedCount++
	}

	completedAt := nowFn().UTC().Format(time.RFC3339)
	if _, err := store.UpsertSyncState(ctx, SyncStateNamespace, SyncStateKey, completedAt); err != nil {
		return SyncResult{}, fmt.Errorf("update Hevy sync state: %w", err)
	}

	return SyncResult{
		Mode:         "full",
		CompletedAt:  completedAt,
		WorkoutCount: len(workouts),
		CreatedCount: createdCount,
		UpdatedCount: updatedCount,
		DeletedCount: deletedCount,
	}, nil
}

func syncEvents(ctx context.Context, store SyncStore, client WorkoutClient, since string, pageSize int, nowFn func() time.Time) (SyncResult, error) {
	events, err := client.IterWorkoutEvents(ctx, since, pageSize)
	if err != nil {
		return SyncResult{}, err
	}
	remoteIndex, err := store.ListActiveWorkoutIDsBySourceType(ctx, SourceType)
	if err != nil {
		return SyncResult{}, fmt.Errorf("list active Hevy workouts: %w", err)
	}

	changedByExternalID := map[string]model.WorkoutCreate{}
	deletedExternalIDs := map[string]struct{}{}
	for _, event := range events {
		eventType := strings.ToLower(strings.TrimSpace(event.Type))
		switch eventType {
		case "updated":
			if event.Workout == nil {
				return SyncResult{}, fmt.Errorf("Hevy updated event missing workout payload")
			}
			payload, err := buildWorkoutCreate(*event.Workout)
			if err != nil {
				return SyncResult{}, err
			}
			externalID := strings.TrimSpace(event.Workout.ID)
			changedByExternalID[externalID] = payload
			delete(deletedExternalIDs, externalID)
		case "deleted":
			externalID := strings.TrimSpace(event.ID)
			if externalID == "" {
				return SyncResult{}, fmt.Errorf("Hevy deleted event missing id")
			}
			delete(changedByExternalID, externalID)
			deletedExternalIDs[externalID] = struct{}{}
		default:
			return SyncResult{}, fmt.Errorf("unsupported Hevy workout event type %q", event.Type)
		}
	}

	createdCount := 0
	updatedCount := 0
	for _, externalID := range sortedExternalIDs(remoteIndexFromChanged(changedByExternalID)) {
		payload := changedByExternalID[externalID]
		trackerID, exists := remoteIndex[externalID]
		if !exists {
			created, err := store.CreateWorkout(ctx, payload)
			if err != nil {
				return SyncResult{}, fmt.Errorf("create Hevy workout %s: %w", externalID, err)
			}
			remoteIndex[externalID] = created.ID
			createdCount++
			continue
		}
		updated, err := store.UpdateWorkout(ctx, trackerID, payload)
		if err != nil {
			return SyncResult{}, fmt.Errorf("update Hevy workout %s: %w", externalID, err)
		}
		remoteIndex[externalID] = updated.ID
		updatedCount++
	}

	deletedCount := 0
	for _, externalID := range sortedDeletedIDs(deletedExternalIDs) {
		trackerID, exists := remoteIndex[externalID]
		if !exists {
			continue
		}
		if err := store.DeleteWorkout(ctx, trackerID); err != nil {
			return SyncResult{}, fmt.Errorf("delete Hevy workout %s: %w", externalID, err)
		}
		deletedCount++
	}

	completedAt := nowFn().UTC().Format(time.RFC3339)
	if _, err := store.UpsertSyncState(ctx, SyncStateNamespace, SyncStateKey, completedAt); err != nil {
		return SyncResult{}, fmt.Errorf("update Hevy sync state: %w", err)
	}

	return SyncResult{
		Mode:         "events",
		Since:        since,
		CompletedAt:  completedAt,
		WorkoutCount: len(changedByExternalID),
		CreatedCount: createdCount,
		UpdatedCount: updatedCount,
		DeletedCount: deletedCount,
	}, nil
}

func buildWorkoutCreate(workout Workout) (model.WorkoutCreate, error) {
	workoutID := strings.TrimSpace(workout.ID)
	if workoutID == "" {
		return model.WorkoutCreate{}, fmt.Errorf("Hevy workout payload missing id")
	}
	startedAt, err := parseRequiredTime(workout.StartTime, "start_time", workoutID)
	if err != nil {
		return model.WorkoutCreate{}, err
	}
	endedAt, err := parseRequiredTime(workout.EndTime, "end_time", workoutID)
	if err != nil {
		return model.WorkoutCreate{}, err
	}

	exercises := make([]model.WorkoutExerciseCreate, 0, len(workout.Exercises))
	for _, exercise := range workout.Exercises {
		displayName := strings.TrimSpace(exercise.Title)
		if displayName == "" {
			displayName = "Untitled exercise"
		}
		_, baseName, modifier := splitExerciseName(displayName)
		sets := make([]model.ExerciseSetCreate, 0, len(exercise.Sets))
		for _, set := range exercise.Sets {
			var distanceKM *float64
			if set.DistanceMeters != nil {
				value := *set.DistanceMeters / 1000.0
				distanceKM = &value
			}
			sets = append(sets, model.ExerciseSetCreate{
				SetNumber:       set.Index + 1,
				SetType:         optionalString(set.Type),
				DistanceKM:      distanceKM,
				WeightKG:        set.WeightKG,
				Reps:            set.Reps,
				DurationSeconds: set.DurationSeconds,
				RPE:             set.RPE,
				CustomMetric:    set.CustomMetric,
				RawPayload:      set,
			})
		}

		exercises = append(exercises, model.WorkoutExerciseCreate{
			OrderIndex:  exercise.Index + 1,
			DisplayName: displayName,
			BaseName:    baseName,
			Modifier:    modifier,
			Notes:       optionalString(exercise.Notes),
			ExternalID:  optionalString(exercise.ExerciseTemplateID),
			RawPayload:  exercise,
			Sets:        sets,
		})
	}

	sourceRef := fmt.Sprintf("https://hevy.com/workout/%s", workoutID)
	return model.WorkoutCreate{
		Title:      firstNonEmpty(workout.Title, "Untitled workout"),
		StartedAt:  startedAt,
		EndedAt:    &endedAt,
		Notes:      optionalString(workout.Description),
		SourceType: SourceType,
		SourceRef:  &sourceRef,
		ExternalID: &workoutID,
		RawPayload: workout,
		Exercises:  exercises,
	}, nil
}

func parseRequiredTime(value string, fieldName string, workoutID string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("Hevy workout %s invalid %s: %w", workoutID, fieldName, err)
	}
	return parsed.UTC(), nil
}

func splitExerciseName(name string) (string, string, *string) {
	if strings.HasSuffix(name, ")") {
		openIndex := strings.LastIndex(name, "(")
		if openIndex > 0 {
			baseName := strings.TrimSpace(name[:openIndex])
			modifier := strings.TrimSpace(name[openIndex+1 : len(name)-1])
			if baseName != "" && modifier != "" {
				return name, baseName, &modifier
			}
		}
	}
	return name, name, nil
}

func optionalString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func firstNonEmpty(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func remoteIndexFromChanged(changed map[string]model.WorkoutCreate) map[string]int64 {
	index := make(map[string]int64, len(changed))
	for externalID := range changed {
		index[externalID] = 0
	}
	return index
}

func sortedDeletedIDs(ids map[string]struct{}) []string {
	result := make([]string, 0, len(ids))
	for externalID := range ids {
		result = append(result, externalID)
	}
	sort.Strings(result)
	return result
}

func sortedExternalIDs(index map[string]int64) []string {
	result := make([]string, 0, len(index))
	for externalID := range index {
		result = append(result, externalID)
	}
	sort.Strings(result)
	return result
}
