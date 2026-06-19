CREATE TABLE IF NOT EXISTS sessions (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    notes TEXT,
    source_type TEXT NOT NULL,
    source_ref TEXT,
    external_id TEXT,
    raw_payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_external_id
    ON sessions (source_type, external_id)
    WHERE external_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_sessions_started_at
    ON sessions (started_at DESC);

CREATE TABLE IF NOT EXISTS session_exercises (
    id BIGSERIAL PRIMARY KEY,
    session_id BIGINT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    order_index INTEGER NOT NULL,
    display_name TEXT NOT NULL,
    base_name TEXT NOT NULL,
    modifier TEXT,
    notes TEXT,
    external_id TEXT,
    raw_payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (session_id, order_index)
);

CREATE INDEX IF NOT EXISTS idx_session_exercises_session_id_order
    ON session_exercises (session_id, order_index);

CREATE INDEX IF NOT EXISTS idx_session_exercises_base_name
    ON session_exercises (base_name);

CREATE TABLE IF NOT EXISTS exercise_sets (
    id BIGSERIAL PRIMARY KEY,
    session_exercise_id BIGINT NOT NULL REFERENCES session_exercises(id) ON DELETE CASCADE,
    set_number INTEGER NOT NULL,
    set_type TEXT,
    distance_km DOUBLE PRECISION,
    weight_kg DOUBLE PRECISION,
    reps DOUBLE PRECISION,
    duration_seconds DOUBLE PRECISION,
    rpe DOUBLE PRECISION,
    custom_metric DOUBLE PRECISION,
    raw_payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (session_exercise_id, set_number),
    CHECK (
        reps IS NOT NULL
        OR duration_seconds IS NOT NULL
        OR distance_km IS NOT NULL
        OR weight_kg IS NOT NULL
    )
);

CREATE INDEX IF NOT EXISTS idx_exercise_sets_session_exercise_id_set_number
    ON exercise_sets (session_exercise_id, set_number);

CREATE TABLE IF NOT EXISTS runs (
    id BIGSERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    sport TEXT NOT NULL,
    sub_sport TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ NOT NULL,
    duration_seconds DOUBLE PRECISION NOT NULL,
    distance_m DOUBLE PRECISION NOT NULL,
    total_calories INTEGER,
    total_ascent_m DOUBLE PRECISION,
    total_descent_m DOUBLE PRECISION,
    start_lat DOUBLE PRECISION,
    start_lon DOUBLE PRECISION,
    end_lat DOUBLE PRECISION,
    end_lon DOUBLE PRECISION,
    source_type TEXT NOT NULL,
    source_ref TEXT,
    external_id TEXT,
    raw_payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_external_id
    ON runs (source_type, external_id)
    WHERE external_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_runs_started_at
    ON runs (started_at DESC);

CREATE TABLE IF NOT EXISTS run_points (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    point_index INTEGER NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    lat DOUBLE PRECISION NOT NULL,
    lon DOUBLE PRECISION NOT NULL,
    altitude_m DOUBLE PRECISION,
    distance_m_from_start DOUBLE PRECISION,
    speed_m_s DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (run_id, point_index)
);

CREATE INDEX IF NOT EXISTS idx_run_points_run_id_point_index
    ON run_points (run_id, point_index);
