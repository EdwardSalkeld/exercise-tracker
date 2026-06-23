package hevy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.hevyapp.com"

type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

type Workout struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	StartTime   string            `json:"start_time"`
	EndTime     string            `json:"end_time"`
	UpdatedAt   string            `json:"updated_at"`
	CreatedAt   string            `json:"created_at"`
	Exercises   []WorkoutExercise `json:"exercises"`
}

type WorkoutExercise struct {
	Index              int           `json:"index"`
	Title              string        `json:"title"`
	Notes              string        `json:"notes"`
	ExerciseTemplateID string        `json:"exercise_template_id"`
	Sets               []ExerciseSet `json:"sets"`
}

type ExerciseSet struct {
	Index           int      `json:"index"`
	Type            string   `json:"type"`
	DistanceMeters  *float64 `json:"distance_meters"`
	WeightKG        *float64 `json:"weight_kg"`
	Reps            *float64 `json:"reps"`
	DurationSeconds *float64 `json:"duration_seconds"`
	RPE             *float64 `json:"rpe"`
	CustomMetric    *float64 `json:"custom_metric"`
}

type WorkoutEvent struct {
	Type      string   `json:"type"`
	ID        string   `json:"id"`
	DeletedAt string   `json:"deleted_at"`
	Workout   *Workout `json:"workout"`
}

type listWorkoutsResponse struct {
	Page      int       `json:"page"`
	PageCount int       `json:"page_count"`
	Workouts  []Workout `json:"workouts"`
}

type listWorkoutEventsResponse struct {
	Page      int            `json:"page"`
	PageCount int            `json:"page_count"`
	Events    []WorkoutEvent `json:"events"`
	Workouts  []Workout      `json:"workouts"`
}

func NewClient(apiKey string, baseURL string) *Client {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) IterWorkouts(ctx context.Context, pageSize int) ([]Workout, error) {
	page := 1
	var items []Workout
	for {
		payload, err := c.listWorkouts(ctx, page, pageSize)
		if err != nil {
			return nil, err
		}
		items = append(items, payload.Workouts...)
		if payload.PageCount <= 0 || page >= payload.PageCount {
			return items, nil
		}
		page++
	}
}

func (c *Client) IterWorkoutEvents(ctx context.Context, since string, pageSize int) ([]WorkoutEvent, error) {
	page := 1
	var items []WorkoutEvent
	for {
		payload, err := c.listWorkoutEvents(ctx, since, page, pageSize)
		if err != nil {
			return nil, err
		}
		events := payload.Events
		if len(events) == 0 && len(payload.Workouts) > 0 {
			events = make([]WorkoutEvent, 0, len(payload.Workouts))
			for i := range payload.Workouts {
				workout := payload.Workouts[i]
				events = append(events, WorkoutEvent{
					Type:    "updated",
					Workout: &workout,
				})
			}
		}
		items = append(items, events...)
		if payload.PageCount <= 0 || page >= payload.PageCount {
			return items, nil
		}
		page++
	}
}

func (c *Client) listWorkouts(ctx context.Context, page int, pageSize int) (listWorkoutsResponse, error) {
	var payload listWorkoutsResponse
	err := c.getJSON(ctx, "/v1/workouts", map[string]string{
		"page":     strconv.Itoa(page),
		"pageSize": strconv.Itoa(pageSize),
	}, &payload)
	if err != nil {
		return listWorkoutsResponse{}, err
	}
	return payload, nil
}

func (c *Client) listWorkoutEvents(ctx context.Context, since string, page int, pageSize int) (listWorkoutEventsResponse, error) {
	var payload listWorkoutEventsResponse
	err := c.getJSON(ctx, "/v1/workouts/events", map[string]string{
		"since":    since,
		"page":     strconv.Itoa(page),
		"pageSize": strconv.Itoa(pageSize),
	}, &payload)
	if err != nil {
		return listWorkoutEventsResponse{}, err
	}
	return payload, nil
}

func (c *Client) getJSON(ctx context.Context, path string, query map[string]string, target any) error {
	requestURL, err := url.Parse(c.baseURL + path)
	if err != nil {
		return fmt.Errorf("build Hevy URL for %s: %w", path, err)
	}
	values := requestURL.Query()
	for key, value := range query {
		values.Set(key, value)
	}
	requestURL.RawQuery = values.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return fmt.Errorf("build Hevy request for %s: %w", path, err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("api-key", c.apiKey)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("Hevy request failed for %s: %w", path, err)
	}
	defer response.Body.Close()

	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read Hevy response for %s: %w", path, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := strings.TrimSpace(string(rawBody))
		message := fmt.Sprintf("Hevy request failed (%s) for %s", response.Status, path)
		if detail != "" {
			message = fmt.Sprintf("%s: %s", message, detail)
		}
		return fmt.Errorf("%s", message)
	}
	if err := json.Unmarshal(rawBody, target); err != nil {
		return fmt.Errorf("decode Hevy response for %s: %w", path, err)
	}
	return nil
}
