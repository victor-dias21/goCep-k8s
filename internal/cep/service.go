package cep

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

const viaCepURL = "https://viacep.com.br/ws/%s/json/"

// ErrInvalidCEP indicates that the provided value does not match the expected CEP format.
var ErrInvalidCEP = errors.New("invalid CEP: expected exactly 8 digits")

// ErrNotFound is returned when neither the cache nor ViaCEP know the requested CEP.
var ErrNotFound = errors.New("cep not found")

// httpClient is the subset of http.Client used by Service, enabling tests with stubs.
type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Response mirrors the JSON structure returned by ViaCEP.
type Response struct {
	Cep         string `json:"cep"`
	Logradouro  string `json:"logradouro"`
	Complemento string `json:"complemento"`
	Bairro      string `json:"bairro"`
	Localidade  string `json:"localidade"`
	Uf          string `json:"uf"`
	Ibge        string `json:"ibge"`
	Gia         string `json:"gia"`
	DDD         string `json:"ddd"`
	Siafi       string `json:"siafi"`
	Unidade     string `json:"unidade"`
	Erro        bool   `json:"erro,omitempty"`
}

// Service fetches CEP details, caching them in PostgreSQL.
type Service struct {
	db        *sql.DB
	client    httpClient
	cacheTTL  time.Duration
	logger    *log.Logger
	now       func() time.Time
	tableName string
}

// NewService builds a Service. cacheTTL <= 0 disables cache expiration.
func NewService(db *sql.DB, client httpClient, cacheTTL time.Duration, logger *log.Logger) *Service {
	if logger == nil {
		logger = log.New(log.Writer(), "", log.LstdFlags)
	}

	return &Service{
		db:        db,
		client:    client,
		cacheTTL:  cacheTTL,
		logger:    logger,
		now:       time.Now,
		tableName: "ceps",
	}
}

// Get retrieves CEP information from cache or ViaCEP.
func (s *Service) Get(ctx context.Context, rawCEP string) (*Response, error) {
	cepDigits, err := normalizeCEP(rawCEP)
	if err != nil {
		return nil, ErrInvalidCEP
	}

	if cached, err := s.loadFromCache(ctx, cepDigits); err != nil {
		return nil, fmt.Errorf("query cache: %w", err)
	} else if cached != nil {
		return cached, nil
	}

	fresh, err := s.fetchFromViaCEP(ctx, cepDigits)
	if err != nil {
		return nil, err
	}

	if err := s.saveToCache(ctx, cepDigits, fresh); err != nil {
		s.logger.Printf("warn: failed to persist cep %s cache: %v", cepDigits, err)
	}

	return fresh, nil
}

// Ping confirms the database connection is alive.
func (s *Service) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Service) loadFromCache(ctx context.Context, cep string) (*Response, error) {
	query := fmt.Sprintf("SELECT payload, updated_at FROM %s WHERE cep = $1", s.tableName)
	row := s.db.QueryRowContext(ctx, query, cep)

	var payload []byte
	var updatedAt time.Time

	switch err := row.Scan(&payload, &updatedAt); {
	case errors.Is(err, sql.ErrNoRows):
		return nil, nil
	case err != nil:
		return nil, err
	}

	if s.cacheTTL > 0 && s.now().Sub(updatedAt) > s.cacheTTL {
		return nil, nil
	}

	var resp Response
	if err := json.Unmarshal(payload, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *Service) saveToCache(ctx context.Context, cep string, data *Response) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (cep, payload, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (cep)
		DO UPDATE SET payload = EXCLUDED.payload, updated_at = EXCLUDED.updated_at
	`, s.tableName)

	_, err = s.db.ExecContext(ctx, query, cep, payload, s.now().UTC())
	return err
}

func (s *Service) fetchFromViaCEP(ctx context.Context, cep string) (*Response, error) {
	url := fmt.Sprintf(viaCepURL, cep)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("viacep returned status %d", resp.StatusCode)
	}

	var body Response
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	if body.Erro {
		return nil, ErrNotFound
	}

	if body.Cep == "" {
		body.Cep = formatCEP(cep)
	}

	return &body, nil
}

// normalizeCEP strips non-digits and validates CEP length.
func normalizeCEP(value string) (string, error) {
	onlyDigits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)

	if len(onlyDigits) != 8 {
		return "", ErrInvalidCEP
	}

	return onlyDigits, nil
}

// formatCEP adds the canonical hyphen to 8-digit CEP strings.
func formatCEP(value string) string {
	if len(value) != 8 {
		return value
	}
	return value[:5] + "-" + value[5:]
}
