package postgres

import (
	"context"
	"testing"

	"chatops-deploy/internal/domain"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestEnsureDevelopmentAdminIsIdempotent(t *testing.T) {
	store := integrationStore(t)

	first, err := store.EnsureDevelopmentAdmin(context.Background())
	require.NoError(t, err)
	second, err := store.EnsureDevelopmentAdmin(context.Background())
	require.NoError(t, err)

	require.Equal(t, first.ID, second.ID)
	principal, err := store.PrincipalForUser(context.Background(), first.ID)
	require.NoError(t, err)
	require.True(t, principal.Allows(domain.ActionAdmin, uuid.Nil))
}

func TestExternalIdentityRoundTrips(t *testing.T) {
	store := integrationStore(t)
	user := createIntegrationUser(t, store)
	identity := domain.ExternalIdentity{UserID: user.ID, Provider: domain.MessageProviderWeCom, SubjectID: "wecom-" + uuid.NewString(), DisplayName: "Integration User"}

	require.NoError(t, store.UpsertExternalIdentity(context.Background(), identity))
	found, err := store.FindUserByIdentity(context.Background(), identity.Provider, identity.SubjectID)
	require.NoError(t, err)
	require.Equal(t, user.ID, found.ID)
	identities, err := store.ListMessageProviderIdentities(context.Background(), user.ID)
	require.NoError(t, err)
	require.Len(t, identities, 1)
	require.Equal(t, identity.UserID, identities[0].UserID)
	require.Equal(t, identity.Provider, identities[0].Provider)
	require.Equal(t, identity.SubjectID, identities[0].SubjectID)
	require.Equal(t, identity.DisplayName, identities[0].DisplayName)
}

func TestActiveProviderRoundTripsAndRejectsUnknownValue(t *testing.T) {
	store := integrationStore(t)

	require.NoError(t, store.SetActiveMessageProvider(context.Background(), domain.MessageProviderDingTalk))
	provider, err := store.GetActiveMessageProvider(context.Background())
	require.NoError(t, err)
	require.Equal(t, domain.MessageProviderDingTalk, provider)
	require.Error(t, store.SetActiveMessageProvider(context.Background(), domain.MessageProvider("telegram")))
}

func TestActiveProviderCannotSwitchWithUnfinishedMessageOperations(t *testing.T) {
	store := integrationStore(t)
	require.NoError(t, store.SetActiveMessageProvider(context.Background(), domain.MessageProviderFeishu))
	createMessageOriginOperation(t, store, domain.MessageProviderFeishu, "chat-1", "event-1")

	require.ErrorIs(t, store.SetActiveMessageProvider(context.Background(), domain.MessageProviderWeCom), domain.ErrConflict)
}

func integrationStore(t *testing.T) *Store {
	t.Helper()
	db := integrationDB(t)
	require.NoError(t, Migrate(context.Background(), db))
	return New(db)
}

func createIntegrationApplication(t *testing.T, store *Store) domain.Application {
	t.Helper()
	app, err := store.CreateApplication(context.Background(), domain.Application{Name: "integration-app-" + uuid.NewString(), Enabled: true})
	require.NoError(t, err)
	return app
}

func createIntegrationCluster(t *testing.T, store *Store) domain.Cluster {
	t.Helper()
	cluster, err := store.CreateCluster(context.Background(), domain.Cluster{ID: uuid.New(), Name: "integration-cluster-" + uuid.NewString(), APIServer: "https://127.0.0.1", EncryptedKubeconfig: []byte("test"), CredentialVersion: 1, Enabled: true})
	require.NoError(t, err)
	return cluster
}

func createIntegrationUser(t *testing.T, store *Store) domain.User {
	t.Helper()
	user, err := store.CreateUser(context.Background(), domain.User{FeishuOpenID: "integration-" + uuid.NewString(), DisplayName: "Integration User", Enabled: true})
	require.NoError(t, err)
	return user
}

func createMessageOriginOperation(t *testing.T, store *Store, provider domain.MessageProvider, conversation, event string) {
	t.Helper()
	app := createIntegrationApplication(t, store)
	cluster := createIntegrationCluster(t, store)
	environment, err := store.CreateEnvironment(context.Background(), domain.AppEnvironment{ApplicationID: app.ID, ClusterID: cluster.ID, Name: "production", Namespace: "default", Deployment: "demo", Container: "app", ImagePrefix: "registry.example/", ApprovalRequired: true})
	require.NoError(t, err)
	requester := createIntegrationUser(t, store)
	_, err = store.CreateOperation(context.Background(), domain.Operation{Kind: domain.OperationDeploy, Status: domain.StatusPendingApproval, ApplicationID: app.ID, EnvironmentID: environment.ID, RequesterID: requester.ID, IdempotencyKey: event, MessageProvider: provider, ConversationID: conversation, EventID: event})
	require.NoError(t, err)
}
