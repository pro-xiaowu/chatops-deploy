package dingtalk_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"chatops-deploy/internal/adapter/dingtalk"
	"chatops-deploy/internal/config"
	"chatops-deploy/internal/messaging"
	"github.com/stretchr/testify/require"
)

func TestProviderDecodesCommandAndVerifiesToken(t *testing.T) {
	provider := dingtalk.NewProvider(config.DingTalkConfig{ClientID: "client", ClientSecret: "secret", RobotCode: "robot", EventToken: "token", APIBaseURL: "https://example.test"})
	body := []byte(`{"msgId":"event-1","conversationId":"chat-1","senderStaffId":"user-1","senderNick":"Alice","text":{"content":"/rollback env-1 3"}}`)

	incoming, err := provider.Decode(context.Background(), messaging.Request{Body: body, Headers: map[string]string{"X-DingTalk-Token": "token"}})

	require.NoError(t, err)
	require.Equal(t, "dingtalk", string(incoming.Provider))
	require.Equal(t, "event-1", incoming.EventID)
	require.Equal(t, "chat-1", incoming.ConversationID)
	require.Equal(t, "user-1", incoming.SubjectID)
	require.Equal(t, "rollback", incoming.Command)
	require.Equal(t, []string{"env-1", "3"}, incoming.Arguments)
}

func TestProviderSendsThroughAccessTokenEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/oauth2/accessToken" {
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "access", "expireIn": 3600})
			return
		}
		require.Equal(t, "/v1.0/robot/oToMessages/batchSend", r.URL.Path)
		require.Equal(t, "access", r.Header.Get("x-acs-dingtalk-access-token"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := dingtalk.NewProvider(config.DingTalkConfig{ClientID: "client", ClientSecret: "secret", RobotCode: "robot", EventToken: "token", APIBaseURL: server.URL})

	require.NoError(t, provider.Send(context.Background(), messaging.Notification{Destination: "user-1", Text: "hello"}))
}

func TestProviderReportsMissingCredentials(t *testing.T) {
	provider := dingtalk.NewProvider(config.DingTalkConfig{})
	require.False(t, provider.Capabilities(context.Background()).Configured)
	require.EqualError(t, provider.Check(context.Background()), "dingtalk provider is not configured")
}
