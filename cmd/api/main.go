package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/victor-dias21/goCep-k8s/internal/cep"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type config struct {
	httpAddr          string
	dbDSN             string
	cacheTTL          time.Duration
	httpClientTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
}

type application struct {
	cfg     config
	logger  *log.Logger
	db      *sql.DB
	service *cep.Service
}

// main bootstraps configuration, dependencies, and starts the HTTP server.
func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	logger := log.New(os.Stdout, "[gocep] ", log.LstdFlags|log.Lshortfile)

	db, err := openDB(cfg.dbDSN)
	if err != nil {
		logger.Fatalf("database error: %v", err)
	}
	defer db.Close()

	if err := prepareDatabase(context.Background(), db); err != nil {
		logger.Fatalf("database migration error: %v", err)
	}

	httpClient := &http.Client{
		Timeout: cfg.httpClientTimeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}

	service := cep.NewService(db, httpClient, cfg.cacheTTL, logger)

	app := &application{
		cfg:     cfg,
		logger:  logger,
		db:      db,
		service: service,
	}

	if err := app.run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("server error: %v", err)
	}
}

func (app *application) run() error {
	router := mux.NewRouter()
	router.HandleFunc("/healthz", app.healthHandler).Methods(http.MethodGet)
	router.HandleFunc("/cep/{cep}", app.cepHandler).Methods(http.MethodGet)

	srv := &http.Server{
		Addr:         app.cfg.httpAddr,
		Handler:      app.logRequests(router),
		ReadTimeout:  app.cfg.readTimeout,
		WriteTimeout: app.cfg.writeTimeout,
		IdleTimeout:  app.cfg.idleTimeout,
	}

	errs := make(chan error, 1)

	go func() {
		app.logger.Printf("API escutando em %s", app.cfg.httpAddr)
		errs <- srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errs:
		return err
	case sig := <-quit:
		app.logger.Printf("recebido sinal %s, iniciando shutdown gracioso", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}

func (app *application) healthHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status := map[string]string{
		"status": "ok",
	}

	if err := app.service.Ping(ctx); err != nil {
		status["status"] = "error"
		status["detail"] = err.Error()
		writeJSON(w, http.StatusServiceUnavailable, status)
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (app *application) cepHandler(w http.ResponseWriter, r *http.Request) {
	params := mux.Vars(r)
	cepValue := params["cep"]

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	result, err := app.service.Get(ctx, cepValue)
	if err != nil {
		switch {
		case errors.Is(err, cep.ErrInvalidCEP):
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		case errors.Is(err, cep.ErrNotFound):
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		default:
			app.logger.Printf("erro ao buscar cep %s: %v", cepValue, err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "falha ao consultar cep"})
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// logRequests logs basic request metadata and latency.
func (app *application) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		duration := time.Since(start)
		app.logger.Printf("%s %s %s", r.Method, r.URL.Path, duration)
	})
}

// loadConfig loads application configuration from environment variables.
func loadConfig() (config, error) {
	cfg := config{
		httpAddr:          getEnvOrDefault("HTTP_ADDR", ":8080"),
		dbDSN:             strings.TrimSpace(os.Getenv("DB_DSN")),
		cacheTTL:          parseDurationOrDefault(os.Getenv("CACHE_TTL"), 24*time.Hour),
		httpClientTimeout: parseDurationOrDefault(os.Getenv("HTTP_CLIENT_TIMEOUT"), 5*time.Second),
		readTimeout:       15 * time.Second,
		writeTimeout:      15 * time.Second,
		idleTimeout:       60 * time.Second,
	}

	if cfg.dbDSN != "" {
		return cfg, nil
	}

	host := strings.TrimSpace(os.Getenv("DB_HOST"))
	port := getEnvOrDefault("DB_PORT", "5432")
	user := strings.TrimSpace(os.Getenv("DB_USER"))
	password := os.Getenv("DB_PASSWORD")
	database := strings.TrimSpace(os.Getenv("DB_NAME"))
	sslMode := getEnvOrDefault("DB_SSLMODE", "disable")

	if host == "" || user == "" || database == "" {
		return cfg, errors.New("DB_DSN não configurado e variáveis de banco incompletas")
	}

	cfg.dbDSN = buildDSN(host, port, user, password, database, sslMode)

	return cfg, nil
}

// parseDurationOrDefault returns a duration or a fallback when parsing fails.
func parseDurationOrDefault(value string, fallback time.Duration) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}

	d, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return d
}

// getEnvOrDefault looks up a trimmed environment variable, falling back when empty.
func getEnvOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

// buildDSN assembles a PostgreSQL DSN from discrete environment settings.
func buildDSN(host, port, user, password, database, sslMode string) string {
	escapedUser := url.QueryEscape(user)
	escapedPassword := url.QueryEscape(password)
	escapedDatabase := url.QueryEscape(database)

	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		escapedUser,
		escapedPassword,
		host,
		port,
		escapedDatabase,
		sslMode,
	)
}

// prepareDatabase ensures the CEP cache table exists before serving requests.
func prepareDatabase(ctx context.Context, db *sql.DB) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS ceps (
	cep TEXT PRIMARY KEY,
	payload JSONB NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`
	_, err := db.ExecContext(ctx, ddl)
	return err
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// writeJSON standardises JSON responses and logs encoding failures.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		// At this point there is no safe way to surface the error to the client,
		// so we log the failure to stderr.
		log.Printf("erro ao escrever resposta json: %v", err)
	}
}
