package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/EdwardSalkeld/exercise-tracker/internal/api"
	"github.com/EdwardSalkeld/exercise-tracker/internal/config"
	"github.com/EdwardSalkeld/exercise-tracker/internal/hevy"
	"github.com/EdwardSalkeld/exercise-tracker/internal/store/postgres"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	args := os.Args[1:]
	command := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		command = args[0]
		args = args[1:]
	}

	switch command {
	case "serve":
		if err := serve(ctx, currentEnv()); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(2)
		}
	case "sync-hevy":
		if err := runHevySync(ctx, currentEnv(), args); err != nil {
			fmt.Fprintf(os.Stderr, "hevy sync error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", command)
		os.Exit(2)
	}
}

func serve(ctx context.Context, environ map[string]string) error {
	cfg, err := config.LoadFromEnv(environ)
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database setup error: %w", err)
	}
	defer store.Close()

	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      api.NewServer(store).Handler(),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}
	return nil
}

func runHevySync(ctx context.Context, environ map[string]string, args []string) error {
	flags := flag.NewFlagSet("sync-hevy", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	full := flags.Bool("full", false, "Fetch the full Hevy workout history and fully reconcile tracker state")
	since := flags.String("since", "", "Override the starting sync timestamp instead of reading sync-state")
	pageSize := flags.Int("page-size", hevy.DefaultPageSize, "Hevy page size")
	apiKey := flags.String("api-key", "", "Hevy API key override")
	baseURL := flags.String("base-url", hevy.DefaultBaseURL, "Hevy API base URL override")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	cfg, err := config.LoadFromEnv(environ)
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	store, err := postgres.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database setup error: %w", err)
	}
	defer store.Close()

	resolvedAPIKey, err := hevy.ResolveAPIKey(*apiKey)
	if err != nil {
		return err
	}

	result, err := hevy.SyncWorkouts(ctx, store, hevy.NewClient(resolvedAPIKey, *baseURL), hevy.SyncOptions{
		Full:     *full,
		Since:    *since,
		PageSize: *pageSize,
	})
	if err != nil {
		return err
	}

	return json.NewEncoder(os.Stdout).Encode(result)
}

func currentEnv() map[string]string {
	result := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}
