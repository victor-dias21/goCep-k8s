package cep

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

type stubHTTPClient struct {
	response *http.Response
	err      error
	calls    int
}

func (s *stubHTTPClient) Do(req *http.Request) (*http.Response, error) {
	s.calls++
	return s.response, s.err
}

func TestNormalizeCEP(t *testing.T) {
	t.Parallel()

	digits, err := normalizeCEP("12345-678")
	assert.NoError(t, err)
	assert.Equal(t, "12345678", digits)

	_, err = normalizeCEP("12-345")
	assert.ErrorIs(t, err, ErrInvalidCEP)
}

func TestServiceGetCacheHit(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	expected := &Response{
		Cep:         "12345-678",
		Logradouro:  "Rua Teste",
		Bairro:      "Centro",
		Localidade:  "Cidade",
		Uf:          "ST",
		Complemento: "",
		Ibge:        "0000000",
	}

	payload, err := json.Marshal(expected)
	assert.NoError(t, err)

	mock.ExpectQuery(`SELECT payload, updated_at FROM ceps WHERE cep = \$1`).
		WithArgs("12345678").
		WillReturnRows(
			sqlmock.NewRows([]string{"payload", "updated_at"}).
				AddRow(payload, time.Now()),
		)

	client := &stubHTTPClient{}
	service := NewService(db, client, time.Hour, noopLogger())

	res, err := service.Get(context.Background(), "12345-678")
	assert.NoError(t, err)
	assert.Equal(t, expected, res)
	assert.Equal(t, 0, client.calls)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestServiceGetCacheMiss(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(`SELECT payload, updated_at FROM ceps WHERE cep = \$1`).
		WithArgs("76543210").
		WillReturnError(sql.ErrNoRows)

	body := `{"cep":"76543-210","logradouro":"Rua Nova","bairro":"Bairro","localidade":"Cidade","uf":"ST","ibge":"1234567"}`

	client := &stubHTTPClient{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}

	mock.ExpectExec(`INSERT INTO ceps`).
		WithArgs("76543210", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	service := NewService(db, client, time.Hour, noopLogger())

	res, err := service.Get(context.Background(), "76543-210")
	assert.NoError(t, err)
	assert.Equal(t, "76543-210", res.Cep)
	assert.Equal(t, 1, client.calls)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestServiceGetRemoteNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectQuery(`SELECT payload, updated_at FROM ceps WHERE cep = \$1`).
		WithArgs("00000000").
		WillReturnError(sql.ErrNoRows)

	client := &stubHTTPClient{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"erro": true}`)),
		},
	}

	service := NewService(db, client, time.Hour, noopLogger())

	_, err = service.Get(context.Background(), "00000000")
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, 1, client.calls)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func noopLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}
