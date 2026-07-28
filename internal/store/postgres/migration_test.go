package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"chatops-deploy/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProviderIdentityValuesRejectUnknownProvider(t *testing.T) {
	require.Error(t, domain.ValidateMessageProvider(domain.MessageProvider("telegram")))
	require.NoError(t, domain.ValidateMessageProvider(domain.MessageProviderWeb))
}

func TestMigrationCreatesProviderSettingsAndExternalIdentities(t *testing.T) {
	db := integrationDB(t)
	require.NoError(t, Migrate(context.Background(), db))
	require.True(t, db.Migrator().HasTable("system_settings"))
	require.True(t, db.Migrator().HasTable("user_external_identities"))
	require.NoError(t, db.Exec("SELECT message_provider, message_conversation_id, message_event_id FROM operations LIMIT 1").Error)
}

func integrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	rawURL := os.Getenv("CHATOPS_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("CHATOPS_TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	adminDB, adminSQL, err := Open(context.Background(), rawURL, 2, 1, time.Minute)
	require.NoError(t, err)
	schema := "test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, adminDB.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error)

	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, sqlDB, err := Open(context.Background(), parsed.String(), 4, 2, time.Minute)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		_ = adminDB.Exec(fmt.Sprintf(`DROP SCHEMA IF EXISTS %q CASCADE`, schema)).Error
		_ = adminSQL.Close()
	})
	return db
}
