package wecom_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"chatops-deploy/internal/adapter/wecom"
	"chatops-deploy/internal/config"
	"chatops-deploy/internal/messaging"
	"github.com/stretchr/testify/require"
)

func TestProviderDecodesCommandAndVerifiesToken(t *testing.T) {
	provider := wecom.NewProvider(config.WeComConfig{CorpID: "corp", AgentID: "1", Secret: "secret", Token: "token", APIBaseURL: "https://example.test"})
	body := []byte(`{"event_id":"event-1","chat_id":"chat-1","userid":"user-1","display_name":"Alice","text":"/deploy env-1 image:v1"}`)

	incoming, err := provider.Decode(context.Background(), messaging.Request{Body: body, Headers: map[string]string{"X-WeCom-Token": "token"}})

	require.NoError(t, err)
	require.Equal(t, "wecom", string(incoming.Provider))
	require.Equal(t, "event-1", incoming.EventID)
	require.Equal(t, "chat-1", incoming.ConversationID)
	require.Equal(t, "user-1", incoming.SubjectID)
	require.Equal(t, "deploy", incoming.Command)
	require.Equal(t, []string{"env-1", "image:v1"}, incoming.Arguments)
}

func TestProviderSendsThroughAccessTokenEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cgi-bin/gettoken" {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "access", "expires_in": 3600})
			return
		}
		require.Equal(t, "/cgi-bin/message/send", r.URL.Path)
		require.Equal(t, "access", r.URL.Query().Get("access_token"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	provider := wecom.NewProvider(config.WeComConfig{CorpID: "corp", AgentID: "1", Secret: "secret", Token: "token", APIBaseURL: server.URL})

	require.NoError(t, provider.Send(context.Background(), messaging.Notification{Destination: "user-1", Text: "hello"}))
}

func TestProviderReportsMissingCredentials(t *testing.T) {
	provider := wecom.NewProvider(config.WeComConfig{})
	require.False(t, provider.Capabilities(context.Background()).Configured)
	require.EqualError(t, provider.Check(context.Background()), "wecom provider is not configured")
}
