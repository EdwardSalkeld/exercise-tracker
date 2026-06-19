package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/EdwardSalkeld/exercise-tracker/internal/api"
	"github.com/EdwardSalkeld/exercise-tracker/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) HealthCheck(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) CreateSession(ctx context.Context, input model.SessionCreate) (model.SessionDetail, error) {
	if err := validateSessionCreate(input); err != nil {
		return model.SessionDetail{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.SessionDetail{}, fmt.Errorf("begin session tx: %w", err)
	}
	defer tx.Rollback(ctx)

	sessionID, err := insertSession(ctx, tx, input)
	if err != nil {
		return model.SessionDetail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.SessionDetail{}, fmt.Errorf("commit session tx: %w", err)
	}

	return s.GetSession(ctx, sessionID)
}

func (s *Store) UpdateSession(ctx context.Context, id int64, input model.SessionCreate) (model.SessionDetail, error) {
	if err := validateSessionCreate(input); err != nil {
		return model.SessionDetail{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.SessionDetail{}, fmt.Errorf("begin session update tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := softDeleteSession(ctx, tx, id); err != nil {
		return model.SessionDetail{}, err
	}

	sessionID, err := insertSession(ctx, tx, input)
	if err != nil {
		return model.SessionDetail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.SessionDetail{}, fmt.Errorf("commit session update tx: %w", err)
	}

	return s.GetSession(ctx, sessionID)
}

func (s *Store) DeleteSession(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE sessions
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("soft delete session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return api.ErrNotFound()
	}
	return nil
}

func (s *Store) ListSessions(ctx context.Context, limit int) ([]model.SessionSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			w.id,
			w.title,
			w.started_at,
			COUNT(DISTINCT we.id) AS exercise_count,
			COUNT(es.id) AS set_count,
			COALESCE(
				SUM(
					CASE
						WHEN es.weight_kg IS NOT NULL AND es.reps IS NOT NULL
							THEN es.weight_kg * es.reps
						ELSE 0
					END
				),
				0
			) AS total_volume_kg
		FROM sessions w
		LEFT JOIN session_exercises we ON we.session_id = w.id
		LEFT JOIN exercise_sets es ON es.session_exercise_id = we.id
		WHERE w.deleted_at IS NULL
		GROUP BY w.id
		ORDER BY w.started_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	var items []model.SessionSummary
	for rows.Next() {
		var item model.SessionSummary
		if err := rows.Scan(
			&item.ID,
			&item.Title,
			&item.StartedAt,
			&item.ExerciseCount,
			&item.SetCount,
			&item.TotalVolumeKG,
		); err != nil {
			return nil, fmt.Errorf("scan session summary: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session summaries: %w", err)
	}
	return items, nil
}

func (s *Store) GetSession(ctx context.Context, id int64) (model.SessionDetail, error) {
	var session model.SessionDetail
	var endedAt sql.NullTime
	var notes sql.NullString
	var sourceRef sql.NullString
	var externalID sql.NullString
	err := s.pool.QueryRow(ctx, `
		SELECT
			w.id,
			w.title,
			w.started_at,
			w.ended_at,
			w.notes,
			w.source_type,
			w.source_ref,
			w.external_id,
			COUNT(DISTINCT we.id) AS exercise_count,
			COUNT(es.id) AS set_count,
			COALESCE(
				SUM(
					CASE
						WHEN es.weight_kg IS NOT NULL AND es.reps IS NOT NULL
							THEN es.weight_kg * es.reps
						ELSE 0
					END
				),
				0
			) AS total_volume_kg
		FROM sessions w
		LEFT JOIN session_exercises we ON we.session_id = w.id
		LEFT JOIN exercise_sets es ON es.session_exercise_id = we.id
		WHERE w.id = $1 AND w.deleted_at IS NULL
		GROUP BY w.id
	`, id).Scan(
		&session.ID,
		&session.Title,
		&session.StartedAt,
		&endedAt,
		&notes,
		&session.SourceType,
		&sourceRef,
		&externalID,
		&session.ExerciseCount,
		&session.SetCount,
		&session.TotalVolumeKG,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.SessionDetail{}, api.ErrNotFound()
		}
		return model.SessionDetail{}, fmt.Errorf("query session: %w", err)
	}
	session.EndedAt = nullTimePtr(endedAt)
	session.Notes = nullStringPtr(notes)
	session.SourceRef = nullStringPtr(sourceRef)
	session.ExternalID = nullStringPtr(externalID)

	rows, err := s.pool.Query(ctx, `
		SELECT
			we.id,
			we.order_index,
			we.display_name,
			we.base_name,
			we.modifier,
			we.notes
		FROM session_exercises we
		WHERE we.session_id = $1
		ORDER BY we.order_index ASC
	`, id)
	if err != nil {
		return model.SessionDetail{}, fmt.Errorf("query session exercises: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var exercise model.SessionExercise
		var modifier sql.NullString
		var notes sql.NullString
		if err := rows.Scan(
			&exercise.ID,
			&exercise.OrderIndex,
			&exercise.DisplayName,
			&exercise.BaseName,
			&modifier,
			&notes,
		); err != nil {
			return model.SessionDetail{}, fmt.Errorf("scan session exercise: %w", err)
		}
		exercise.Modifier = nullStringPtr(modifier)
		exercise.Notes = nullStringPtr(notes)
		exercise.Sets, err = s.loadExerciseSets(ctx, exercise.ID)
		if err != nil {
			return model.SessionDetail{}, err
		}
		session.Exercises = append(session.Exercises, exercise)
	}
	if err := rows.Err(); err != nil {
		return model.SessionDetail{}, fmt.Errorf("iterate session exercises: %w", err)
	}

	return session, nil
}

func (s *Store) loadExerciseSets(ctx context.Context, exerciseID int64) ([]model.ExerciseSet, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id,
			set_number,
			set_type,
			distance_km,
			weight_kg,
			reps,
			duration_seconds,
			rpe,
			custom_metric
		FROM exercise_sets
		WHERE session_exercise_id = $1
		ORDER BY set_number ASC
	`, exerciseID)
	if err != nil {
		return nil, fmt.Errorf("query exercise sets: %w", err)
	}
	defer rows.Close()

	var sets []model.ExerciseSet
	for rows.Next() {
		var item model.ExerciseSet
		var setType sql.NullString
		var distanceKM sql.NullFloat64
		var weightKG sql.NullFloat64
		var reps sql.NullFloat64
		var durationSeconds sql.NullFloat64
		var rpe sql.NullFloat64
		var customMetric sql.NullFloat64
		if err := rows.Scan(
			&item.ID,
			&item.SetNumber,
			&setType,
			&distanceKM,
			&weightKG,
			&reps,
			&durationSeconds,
			&rpe,
			&customMetric,
		); err != nil {
			return nil, fmt.Errorf("scan exercise set: %w", err)
		}
		item.SetType = nullStringPtr(setType)
		item.DistanceKM = nullFloat64Ptr(distanceKM)
		item.WeightKG = nullFloat64Ptr(weightKG)
		item.Reps = nullFloat64Ptr(reps)
		item.DurationSeconds = nullFloat64Ptr(durationSeconds)
		item.RPE = nullFloat64Ptr(rpe)
		item.CustomMetric = nullFloat64Ptr(customMetric)
		sets = append(sets, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exercise sets: %w", err)
	}
	return sets, nil
}

func (s *Store) ExerciseHistory(ctx context.Context, baseName string, limit int) ([]model.ExerciseHistoryItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			w.id,
			w.title,
			w.started_at,
			we.display_name,
			es.set_number,
			es.distance_km,
			es.weight_kg,
			es.reps,
			es.duration_seconds
		FROM sessions w
		JOIN session_exercises we ON we.session_id = w.id
		JOIN exercise_sets es ON es.session_exercise_id = we.id
		WHERE we.base_name = $1 AND w.deleted_at IS NULL
		ORDER BY w.started_at DESC, es.set_number ASC
		LIMIT $2
	`, baseName, limit)
	if err != nil {
		return nil, fmt.Errorf("query exercise history: %w", err)
	}
	defer rows.Close()

	var items []model.ExerciseHistoryItem
	for rows.Next() {
		var item model.ExerciseHistoryItem
		var distanceKM sql.NullFloat64
		var weightKG sql.NullFloat64
		var reps sql.NullFloat64
		var durationSeconds sql.NullFloat64
		if err := rows.Scan(
			&item.SessionID,
			&item.SessionTitle,
			&item.SessionStartedAt,
			&item.DisplayName,
			&item.SetNumber,
			&distanceKM,
			&weightKG,
			&reps,
			&durationSeconds,
		); err != nil {
			return nil, fmt.Errorf("scan exercise history: %w", err)
		}
		item.DistanceKM = nullFloat64Ptr(distanceKM)
		item.WeightKG = nullFloat64Ptr(weightKG)
		item.Reps = nullFloat64Ptr(reps)
		item.DurationSeconds = nullFloat64Ptr(durationSeconds)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate exercise history: %w", err)
	}
	return items, nil
}

func (s *Store) CreateRun(ctx context.Context, input model.RunCreate) (model.RunDetail, error) {
	if err := validateRunCreate(input); err != nil {
		return model.RunDetail{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.RunDetail{}, fmt.Errorf("begin run tx: %w", err)
	}
	defer tx.Rollback(ctx)

	runID, err := insertRun(ctx, tx, input)
	if err != nil {
		return model.RunDetail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.RunDetail{}, fmt.Errorf("commit run tx: %w", err)
	}

	return s.GetRun(ctx, runID)
}

func (s *Store) UpdateRun(ctx context.Context, id int64, input model.RunCreate) (model.RunDetail, error) {
	if err := validateRunCreate(input); err != nil {
		return model.RunDetail{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.RunDetail{}, fmt.Errorf("begin run update tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := softDeleteRun(ctx, tx, id); err != nil {
		return model.RunDetail{}, err
	}

	runID, err := insertRun(ctx, tx, input)
	if err != nil {
		return model.RunDetail{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return model.RunDetail{}, fmt.Errorf("commit run update tx: %w", err)
	}

	return s.GetRun(ctx, runID)
}

func (s *Store) DeleteRun(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE runs
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("soft delete run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return api.ErrNotFound()
	}
	return nil
}

func (s *Store) ListRuns(ctx context.Context, limit int) ([]model.RunSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			id,
			title,
			sport,
			sub_sport,
			started_at,
			ended_at,
			duration_seconds,
			distance_m
		FROM runs
		WHERE deleted_at IS NULL
		ORDER BY started_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("query runs: %w", err)
	}
	defer rows.Close()

	var items []model.RunSummary
	for rows.Next() {
		var item model.RunSummary
		var subSport sql.NullString
		if err := rows.Scan(
			&item.ID,
			&item.Title,
			&item.Sport,
			&subSport,
			&item.StartedAt,
			&item.EndedAt,
			&item.DurationSeconds,
			&item.DistanceM,
		); err != nil {
			return nil, fmt.Errorf("scan run summary: %w", err)
		}
		item.SubSport = nullStringPtr(subSport)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate run summaries: %w", err)
	}
	return items, nil
}

func (s *Store) GetRun(ctx context.Context, id int64) (model.RunDetail, error) {
	var run model.RunDetail
	var subSport sql.NullString
	var totalCalories sql.NullInt64
	var totalAscentM sql.NullFloat64
	var totalDescentM sql.NullFloat64
	var startLat sql.NullFloat64
	var startLon sql.NullFloat64
	var endLat sql.NullFloat64
	var endLon sql.NullFloat64
	var sourceRef sql.NullString
	var externalID sql.NullString
	err := s.pool.QueryRow(ctx, `
		SELECT
			id,
			title,
			sport,
			sub_sport,
			started_at,
			ended_at,
			duration_seconds,
			distance_m,
			total_calories,
			total_ascent_m,
			total_descent_m,
			start_lat,
			start_lon,
			end_lat,
			end_lon,
			source_type,
			source_ref,
			external_id
		FROM runs
		WHERE id = $1 AND deleted_at IS NULL
	`, id).Scan(
		&run.ID,
		&run.Title,
		&run.Sport,
		&subSport,
		&run.StartedAt,
		&run.EndedAt,
		&run.DurationSeconds,
		&run.DistanceM,
		&totalCalories,
		&totalAscentM,
		&totalDescentM,
		&startLat,
		&startLon,
		&endLat,
		&endLon,
		&run.SourceType,
		&sourceRef,
		&externalID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.RunDetail{}, api.ErrNotFound()
		}
		return model.RunDetail{}, fmt.Errorf("query run: %w", err)
	}
	run.SubSport = nullStringPtr(subSport)
	run.TotalCalories = nullIntPtr(totalCalories)
	run.TotalAscentM = nullFloat64Ptr(totalAscentM)
	run.TotalDescentM = nullFloat64Ptr(totalDescentM)
	run.StartLat = nullFloat64Ptr(startLat)
	run.StartLon = nullFloat64Ptr(startLon)
	run.EndLat = nullFloat64Ptr(endLat)
	run.EndLon = nullFloat64Ptr(endLon)
	run.SourceRef = nullStringPtr(sourceRef)
	run.ExternalID = nullStringPtr(externalID)

	rows, err := s.pool.Query(ctx, `
		SELECT
			point_index,
			recorded_at,
			lat,
			lon,
			altitude_m,
			distance_m_from_start,
			speed_m_s
		FROM run_points
		WHERE run_id = $1
		ORDER BY point_index ASC
	`, id)
	if err != nil {
		return model.RunDetail{}, fmt.Errorf("query run points: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var point model.RunPoint
		var altitudeM sql.NullFloat64
		var distanceMFromStart sql.NullFloat64
		var speedMS sql.NullFloat64
		if err := rows.Scan(
			&point.PointIndex,
			&point.RecordedAt,
			&point.Lat,
			&point.Lon,
			&altitudeM,
			&distanceMFromStart,
			&speedMS,
		); err != nil {
			return model.RunDetail{}, fmt.Errorf("scan run point: %w", err)
		}
		point.AltitudeM = nullFloat64Ptr(altitudeM)
		point.DistanceMFromStart = nullFloat64Ptr(distanceMFromStart)
		point.SpeedMS = nullFloat64Ptr(speedMS)
		run.Points = append(run.Points, point)
	}
	if err := rows.Err(); err != nil {
		return model.RunDetail{}, fmt.Errorf("iterate run points: %w", err)
	}

	return run, nil
}

func insertSession(ctx context.Context, tx pgx.Tx, input model.SessionCreate) (int64, error) {
	rawPayload, err := marshalNullableJSON(input.RawPayload)
	if err != nil {
		return 0, err
	}

	var sessionID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO sessions (
			title,
			started_at,
			ended_at,
			notes,
			source_type,
			source_ref,
			external_id,
			raw_payload
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id
	`,
		strings.TrimSpace(input.Title),
		input.StartedAt,
		input.EndedAt,
		nullableStringValue(input.Notes),
		strings.TrimSpace(input.SourceType),
		nullableStringValue(input.SourceRef),
		nullableStringValue(input.ExternalID),
		rawPayload,
	).Scan(&sessionID)
	if err != nil {
		return 0, fmt.Errorf("insert session: %w", err)
	}

	for i, exercise := range input.Exercises {
		exercisePayload, err := marshalNullableJSON(exercise.RawPayload)
		if err != nil {
			return 0, err
		}
		orderIndex := exercise.OrderIndex
		if orderIndex <= 0 {
			orderIndex = i + 1
		}

		var exerciseID int64
		err = tx.QueryRow(ctx, `
			INSERT INTO session_exercises (
				session_id,
				order_index,
				display_name,
				base_name,
				modifier,
				notes,
				external_id,
				raw_payload
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id
		`,
			sessionID,
			orderIndex,
			strings.TrimSpace(exercise.DisplayName),
			strings.TrimSpace(exercise.BaseName),
			nullableStringValue(exercise.Modifier),
			nullableStringValue(exercise.Notes),
			nullableStringValue(exercise.ExternalID),
			exercisePayload,
		).Scan(&exerciseID)
		if err != nil {
			return 0, fmt.Errorf("insert session exercise: %w", err)
		}

		for j, set := range exercise.Sets {
			setPayload, err := marshalNullableJSON(set.RawPayload)
			if err != nil {
				return 0, err
			}
			setNumber := set.SetNumber
			if setNumber <= 0 {
				setNumber = j + 1
			}

			_, err = tx.Exec(ctx, `
				INSERT INTO exercise_sets (
					session_exercise_id,
					set_number,
					set_type,
					distance_km,
					weight_kg,
					reps,
					duration_seconds,
					rpe,
					custom_metric,
					raw_payload
				)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			`,
				exerciseID,
				setNumber,
				nullableStringValue(set.SetType),
				set.DistanceKM,
				set.WeightKG,
				set.Reps,
				set.DurationSeconds,
				set.RPE,
				set.CustomMetric,
				setPayload,
			)
			if err != nil {
				return 0, fmt.Errorf("insert exercise set: %w", err)
			}
		}
	}

	return sessionID, nil
}

func insertRun(ctx context.Context, tx pgx.Tx, input model.RunCreate) (int64, error) {
	rawPayload, err := marshalNullableJSON(input.RawPayload)
	if err != nil {
		return 0, err
	}

	var runID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO runs (
			title,
			sport,
			sub_sport,
			started_at,
			ended_at,
			duration_seconds,
			distance_m,
			total_calories,
			total_ascent_m,
			total_descent_m,
			start_lat,
			start_lon,
			end_lat,
			end_lon,
			source_type,
			source_ref,
			external_id,
			raw_payload
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		RETURNING id
	`,
		strings.TrimSpace(input.Title),
		strings.TrimSpace(input.Sport),
		nullableStringValue(input.SubSport),
		input.StartedAt,
		input.EndedAt,
		input.DurationSeconds,
		input.DistanceM,
		input.TotalCalories,
		input.TotalAscentM,
		input.TotalDescentM,
		input.StartLat,
		input.StartLon,
		input.EndLat,
		input.EndLon,
		strings.TrimSpace(input.SourceType),
		nullableStringValue(input.SourceRef),
		nullableStringValue(input.ExternalID),
		rawPayload,
	).Scan(&runID)
	if err != nil {
		return 0, fmt.Errorf("insert run: %w", err)
	}

	for i, point := range input.Points {
		pointIndex := point.PointIndex
		if pointIndex <= 0 {
			pointIndex = i + 1
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO run_points (
				run_id,
				point_index,
				recorded_at,
				lat,
				lon,
				altitude_m,
				distance_m_from_start,
				speed_m_s
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`,
			runID,
			pointIndex,
			point.RecordedAt,
			point.Lat,
			point.Lon,
			point.AltitudeM,
			point.DistanceMFromStart,
			point.SpeedMS,
		)
		if err != nil {
			return 0, fmt.Errorf("insert run point: %w", err)
		}
	}

	return runID, nil
}

func softDeleteSession(ctx context.Context, tx pgx.Tx, id int64) error {
	tag, err := tx.Exec(ctx, `
		UPDATE sessions
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("soft delete session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return api.ErrNotFound()
	}
	return nil
}

func softDeleteRun(ctx context.Context, tx pgx.Tx, id int64) error {
	tag, err := tx.Exec(ctx, `
		UPDATE runs
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("soft delete run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return api.ErrNotFound()
	}
	return nil
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullFloat64Ptr(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	return &value.Float64
}

func nullIntPtr(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int64)
	return &result
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func validateSessionCreate(input model.SessionCreate) error {
	if strings.TrimSpace(input.Title) == "" {
		return fmt.Errorf("%w: title is required", api.ErrInvalidInput())
	}
	if input.StartedAt.IsZero() {
		return fmt.Errorf("%w: started_at is required", api.ErrInvalidInput())
	}
	if strings.TrimSpace(input.SourceType) == "" {
		return fmt.Errorf("%w: source_type is required", api.ErrInvalidInput())
	}
	if len(input.Exercises) == 0 {
		return fmt.Errorf("%w: at least one exercise is required", api.ErrInvalidInput())
	}
	for i, exercise := range input.Exercises {
		if strings.TrimSpace(exercise.DisplayName) == "" {
			return fmt.Errorf("%w: exercises[%d].display_name is required", api.ErrInvalidInput(), i)
		}
		if strings.TrimSpace(exercise.BaseName) == "" {
			return fmt.Errorf("%w: exercises[%d].base_name is required", api.ErrInvalidInput(), i)
		}
		if len(exercise.Sets) == 0 {
			return fmt.Errorf("%w: exercises[%d].sets must contain at least one set", api.ErrInvalidInput(), i)
		}
		for j, set := range exercise.Sets {
			if set.Reps == nil && set.DurationSeconds == nil && set.DistanceKM == nil && set.WeightKG == nil {
				return fmt.Errorf("%w: exercises[%d].sets[%d] must include reps, duration_seconds, distance_km, or weight_kg", api.ErrInvalidInput(), i, j)
			}
		}
	}
	return nil
}

func validateRunCreate(input model.RunCreate) error {
	if strings.TrimSpace(input.Title) == "" {
		return fmt.Errorf("%w: title is required", api.ErrInvalidInput())
	}
	if strings.TrimSpace(input.Sport) == "" {
		return fmt.Errorf("%w: sport is required", api.ErrInvalidInput())
	}
	if input.StartedAt.IsZero() {
		return fmt.Errorf("%w: started_at is required", api.ErrInvalidInput())
	}
	if input.EndedAt.IsZero() {
		return fmt.Errorf("%w: ended_at is required", api.ErrInvalidInput())
	}
	if input.DurationSeconds <= 0 {
		return fmt.Errorf("%w: duration_seconds must be positive", api.ErrInvalidInput())
	}
	if input.DistanceM < 0 {
		return fmt.Errorf("%w: distance_m must be zero or greater", api.ErrInvalidInput())
	}
	if strings.TrimSpace(input.SourceType) == "" {
		return fmt.Errorf("%w: source_type is required", api.ErrInvalidInput())
	}
	return nil
}

func nullableStringValue(value *string) any {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

func marshalNullableJSON(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal raw_payload: %w", err)
	}
	return payload, nil
}
