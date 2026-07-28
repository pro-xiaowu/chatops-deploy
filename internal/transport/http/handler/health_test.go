package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	transporthttp "chatops-deploy/internal/transport/http"
	"github.com/stretchr/testify/require"
)

func TestHealthzReturnsOK(t *testing.T) {
	router := transporthttp.NewRouter(transporthttp.Dependencies{
		Readiness: func(context.Context) error { return nil },
	})
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"data":{"status":"ok"}}`, recorder.Body.String())
}

func TestReadyzReturnsOKWhenDependenciesAreReady(t *testing.T) {
	router := transporthttp.NewRouter(transporthttp.Dependencies{
		Readiness: func(context.Context) error { return nil },
	})
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"data":{"status":"ok"}}`, recorder.Body.String())
}

func TestReadyzReturnsStableErrorWithoutLeakingDependencyFailure(t *testing.T) {
	router := transporthttp.NewRouter(transporthttp.Dependencies{
		Readiness: func(context.Context) error { return errors.New("password authentication failed for db-host") },
	})
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.JSONEq(t, `{"error":{"code":"service_unavailable","message":"service is not ready"}}`, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), "password")
	require.NotContains(t, recorder.Body.String(), "db-host")
}
