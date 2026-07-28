package worker

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/messaging"
	storepostgres "chatops-deploy/internal/store/postgres"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type recordingProvider struct{ sent int }

func (*recordingProvider) Name() domain.MessageProvider { return domain.MessageProviderWeCom }
func (*recordingProvider) Capabilities(context.Context) messaging.Capabilities {
	return messaging.Capabilities{Provider: domain.MessageProviderWeCom, Configured: true}
}
func (*recordingProvider) Decode(context.Context, messaging.Request) (messaging.Incoming, error) {
	return messaging.Incoming{}, messaging.ErrUnsupportedDecode
}
func (p *recordingProvider) Send(context.Context, messaging.Notification) error {
	p.sent++
	return nil
}
func (*recordingProvider) Check(context.Context) error { return nil }

func TestDispatchNotificationsClaimsEachMessageOnce(t *testing.T) {
	store := workerIntegrationStore(t)
	provider := &recordingProvider{}
	registry := messaging.NewRegistry(store, []messaging.Provider{provider})
	require.NoError(t, store.EnqueueNotification(context.Background(), messaging.Notification{OperationID: uuid.NewString(), Destination: "user-1", Text: "done"}, domain.MessageProviderWeCom))
	worker := New("worker-1", time.Second, store, nil, registry, zap.NewNop())

	worker.dispatchNotifications(context.Background())
	worker.dispatchNotifications(context.Background())

	require.Equal(t, 1, provider.sent)
	rows, err := store.PendingOutbox(context.Background(), 10)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func workerIntegrationStore(t *testing.T) *storepostgres.Store {
	t.Helper()
	rawURL := os.Getenv("CHATOPS_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("CHATOPS_TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	adminDB, adminSQL, err := storepostgres.Open(context.Background(), rawURL, 2, 1, time.Minute)
	require.NoError(t, err)
	schema := "worker_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, adminDB.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error)
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, sqlDB, err := storepostgres.Open(context.Background(), parsed.String(), 4, 2, time.Minute)
	require.NoError(t, err)
	require.NoError(t, storepostgres.Migrate(context.Background(), db))
	t.Cleanup(func() {
		_ = sqlDB.Close()
		_ = adminDB.Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE`, schema)).Error
		_ = adminSQL.Close()
	})
	return storepostgres.New(db)
}
