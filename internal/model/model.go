package model

import "time"

type SessionSummary struct {
	ID            int64     `json:"id"`
	Title         string    `json:"title"`
	StartedAt     time.Time `json:"started_at"`
	ExerciseCount int       `json:"exercise_count"`
	SetCount      int       `json:"set_count"`
	TotalVolumeKG float64   `json:"total_volume_kg"`
}

type ExerciseSet struct {
	ID              int64    `json:"id"`
	SetNumber       int      `json:"set_number"`
	SetType         *string  `json:"set_type,omitempty"`
	DistanceKM      *float64 `json:"distance_km,omitempty"`
	WeightKG        *float64 `json:"weight_kg,omitempty"`
	Reps            *float64 `json:"reps,omitempty"`
	DurationSeconds *float64 `json:"duration_seconds,omitempty"`
	RPE             *float64 `json:"rpe,omitempty"`
	CustomMetric    *float64 `json:"custom_metric,omitempty"`
}

type SessionExercise struct {
	ID          int64         `json:"id"`
	OrderIndex  int           `json:"order_index"`
	DisplayName string        `json:"display_name"`
	BaseName    string        `json:"base_name"`
	Modifier    *string       `json:"modifier,omitempty"`
	Notes       *string       `json:"notes,omitempty"`
	Sets        []ExerciseSet `json:"sets"`
}

type SessionDetail struct {
	SessionSummary
	EndedAt    *time.Time        `json:"ended_at,omitempty"`
	Notes      *string           `json:"notes,omitempty"`
	SourceType string            `json:"source_type"`
	SourceRef  *string           `json:"source_ref,omitempty"`
	ExternalID *string           `json:"external_id,omitempty"`
	Exercises  []SessionExercise `json:"exercises"`
}

type SessionCreate struct {
	Title      string                  `json:"title"`
	StartedAt  time.Time               `json:"started_at"`
	EndedAt    *time.Time              `json:"ended_at,omitempty"`
	Notes      *string                 `json:"notes,omitempty"`
	SourceType string                  `json:"source_type"`
	SourceRef  *string                 `json:"source_ref,omitempty"`
	ExternalID *string                 `json:"external_id,omitempty"`
	RawPayload any                     `json:"raw_payload,omitempty"`
	Exercises  []SessionExerciseCreate `json:"exercises"`
}

type SessionExerciseCreate struct {
	OrderIndex  int                 `json:"order_index,omitempty"`
	DisplayName string              `json:"display_name"`
	BaseName    string              `json:"base_name"`
	Modifier    *string             `json:"modifier,omitempty"`
	Notes       *string             `json:"notes,omitempty"`
	ExternalID  *string             `json:"external_id,omitempty"`
	RawPayload  any                 `json:"raw_payload,omitempty"`
	Sets        []ExerciseSetCreate `json:"sets"`
}

type ExerciseSetCreate struct {
	SetNumber       int      `json:"set_number,omitempty"`
	SetType         *string  `json:"set_type,omitempty"`
	DistanceKM      *float64 `json:"distance_km,omitempty"`
	WeightKG        *float64 `json:"weight_kg,omitempty"`
	Reps            *float64 `json:"reps,omitempty"`
	DurationSeconds *float64 `json:"duration_seconds,omitempty"`
	RPE             *float64 `json:"rpe,omitempty"`
	CustomMetric    *float64 `json:"custom_metric,omitempty"`
	RawPayload      any      `json:"raw_payload,omitempty"`
}

type ExerciseHistoryItem struct {
	SessionID        int64     `json:"session_id"`
	SessionTitle     string    `json:"session_title"`
	SessionStartedAt time.Time `json:"session_started_at"`
	DisplayName      string    `json:"display_name"`
	SetNumber        int       `json:"set_number"`
	DistanceKM       *float64  `json:"distance_km,omitempty"`
	WeightKG         *float64  `json:"weight_kg,omitempty"`
	Reps             *float64  `json:"reps,omitempty"`
	DurationSeconds  *float64  `json:"duration_seconds,omitempty"`
}

type RunSummary struct {
	ID              int64     `json:"id"`
	Title           string    `json:"title"`
	Sport           string    `json:"sport"`
	SubSport        *string   `json:"sub_sport,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	DurationSeconds float64   `json:"duration_seconds"`
	DistanceM       float64   `json:"distance_m"`
}

type RunPoint struct {
	PointIndex         int       `json:"point_index"`
	RecordedAt         time.Time `json:"recorded_at"`
	Lat                float64   `json:"lat"`
	Lon                float64   `json:"lon"`
	AltitudeM          *float64  `json:"altitude_m,omitempty"`
	DistanceMFromStart *float64  `json:"distance_m_from_start,omitempty"`
	SpeedMS            *float64  `json:"speed_m_s,omitempty"`
}

type RunDetail struct {
	RunSummary
	TotalCalories *int       `json:"total_calories,omitempty"`
	TotalAscentM  *float64   `json:"total_ascent_m,omitempty"`
	TotalDescentM *float64   `json:"total_descent_m,omitempty"`
	StartLat      *float64   `json:"start_lat,omitempty"`
	StartLon      *float64   `json:"start_lon,omitempty"`
	EndLat        *float64   `json:"end_lat,omitempty"`
	EndLon        *float64   `json:"end_lon,omitempty"`
	SourceType    string     `json:"source_type"`
	SourceRef     *string    `json:"source_ref,omitempty"`
	ExternalID    *string    `json:"external_id,omitempty"`
	Points        []RunPoint `json:"points"`
}

type RunCreate struct {
	Title           string           `json:"title"`
	Sport           string           `json:"sport"`
	SubSport        *string          `json:"sub_sport,omitempty"`
	StartedAt       time.Time        `json:"started_at"`
	EndedAt         time.Time        `json:"ended_at"`
	DurationSeconds float64          `json:"duration_seconds"`
	DistanceM       float64          `json:"distance_m"`
	TotalCalories   *int             `json:"total_calories,omitempty"`
	TotalAscentM    *float64         `json:"total_ascent_m,omitempty"`
	TotalDescentM   *float64         `json:"total_descent_m,omitempty"`
	StartLat        *float64         `json:"start_lat,omitempty"`
	StartLon        *float64         `json:"start_lon,omitempty"`
	EndLat          *float64         `json:"end_lat,omitempty"`
	EndLon          *float64         `json:"end_lon,omitempty"`
	SourceType      string           `json:"source_type"`
	SourceRef       *string          `json:"source_ref,omitempty"`
	ExternalID      *string          `json:"external_id,omitempty"`
	RawPayload      any              `json:"raw_payload,omitempty"`
	Points          []RunPointCreate `json:"points"`
}

type RunPointCreate struct {
	PointIndex         int       `json:"point_index,omitempty"`
	RecordedAt         time.Time `json:"recorded_at"`
	Lat                float64   `json:"lat"`
	Lon                float64   `json:"lon"`
	AltitudeM          *float64  `json:"altitude_m,omitempty"`
	DistanceMFromStart *float64  `json:"distance_m_from_start,omitempty"`
	SpeedMS            *float64  `json:"speed_m_s,omitempty"`
}
