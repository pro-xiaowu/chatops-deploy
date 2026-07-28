package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/messaging"
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
	_ = createMessageOriginOperation(t, store, domain.MessageProviderFeishu, "chat-1", "event-1")

	require.ErrorIs(t, store.SetActiveMessageProvider(context.Background(), domain.MessageProviderWeCom), domain.ErrConflict)
}

func TestOperationTransitionAndNotificationAreAtomic(t *testing.T) {
	store := integrationStore(t)
	operation := createMessageOriginOperation(t, store, domain.MessageProviderWeCom, "conversation-1", "event-1")
	require.NoError(t, store.DB.Model(&OperationModel{}).Where("id = ?", operation.ID).Update("status", string(domain.StatusRunning)).Error)
	notification := messaging.Notification{OperationID: operation.ID.String(), Destination: "conversation-1", Text: "done"}

	require.NoError(t, store.TransitionOperationWithNotification(context.Background(), operation.ID, domain.StatusRunning, domain.StatusSucceeded, "", "", &notification, domain.MessageProviderWeCom))

	updated, err := store.GetOperation(context.Background(), operation.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatusSucceeded, updated.Status)
	rows, err := store.PendingOutbox(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, operation.ID.String(), notification.OperationID)
}

func TestNotificationOutboxRetriesAndCompletes(t *testing.T) {
	store := integrationStore(t)
	notification := messaging.Notification{OperationID: uuid.NewString(), Destination: "conversation-1", Text: "done"}

	require.NoError(t, store.EnqueueNotification(context.Background(), notification, domain.MessageProviderWeCom))
	rows, err := store.ClaimOutbox(context.Background(), 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, string(domain.MessageProviderWeCom), rows[0].Provider)
	require.JSONEq(t, `{"OperationID":"`+notification.OperationID+`","Destination":"conversation-1","Text":"done","ApprovalCard":null}`, string(rows[0].Payload))
	claimedAgain, err := store.ClaimOutbox(context.Background(), 10, time.Minute)
	require.NoError(t, err)
	require.Empty(t, claimedAgain)

	require.NoError(t, store.MarkOutboxFailed(context.Background(), rows[0].ID, errors.New("temporary failure")))
	var failed OutboxModel
	require.NoError(t, store.DB.First(&failed, "id = ?", rows[0].ID).Error)
	require.Equal(t, 1, failed.Attempts)
	require.Equal(t, "temporary failure", failed.LastError)

	require.NoError(t, store.MarkOutboxSent(context.Background(), rows[0].ID))
	var sent OutboxModel
	require.NoError(t, store.DB.First(&sent, "id = ?", rows[0].ID).Error)
	require.NotNil(t, sent.SentAt)
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

func createMessageOriginOperation(t *testing.T, store *Store, provider domain.MessageProvider, conversation, event string) domain.Operation {
	t.Helper()
	app := createIntegrationApplication(t, store)
	cluster := createIntegrationCluster(t, store)
	environment, err := store.CreateEnvironment(context.Background(), domain.AppEnvironment{ApplicationID: app.ID, ClusterID: cluster.ID, Name: "production", Namespace: "default", Deployment: "demo", Container: "app", ImagePrefix: "registry.example/", ApprovalRequired: true})
	require.NoError(t, err)
	requester := createIntegrationUser(t, store)
	operation, err := store.CreateOperation(context.Background(), domain.Operation{Kind: domain.OperationDeploy, Status: domain.StatusPendingApproval, ApplicationID: app.ID, EnvironmentID: environment.ID, RequesterID: requester.ID, IdempotencyKey: event, MessageProvider: provider, ConversationID: conversation, EventID: event})
	require.NoError(t, err)
	return operation
}
