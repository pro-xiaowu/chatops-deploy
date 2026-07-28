package feishu_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"chatops-deploy/internal/adapter/feishu"
	"chatops-deploy/internal/config"
	"chatops-deploy/internal/messaging"
	"github.com/stretchr/testify/require"
)

func TestProviderDecodesCommandAndApproval(t *testing.T) {
	provider := feishu.NewProvider(config.FeishuConfig{AppID: "app", AppSecret: "secret", APIBaseURL: "https://example.test", VerificationToken: "verify"})
	body := []byte(`{"header":{"event_id":"event-1","token":"verify"},"event":{"sender":{"sender_id":{"open_id":"ou-user"}},"message":{"chat_id":"oc-chat","content":"{\"text\":\"/deploy env-1 image:v1\"}"},"action":{"value":{"operation_id":"op-1","decision":"approved"}}}}`)

	incoming, err := provider.Decode(context.Background(), messaging.Request{Body: body, Headers: map[string]string{}})

	require.NoError(t, err)
	require.Equal(t, "feishu", string(incoming.Provider))
	require.Equal(t, "event-1", incoming.EventID)
	require.Equal(t, "oc-chat", incoming.ConversationID)
	require.Equal(t, "ou-user", incoming.SubjectID)
	require.Equal(t, "deploy", incoming.Command)
	require.Equal(t, []string{"env-1", "image:v1"}, incoming.Arguments)
	require.Equal(t, "op-1", incoming.OperationID)
	require.Equal(t, "approved", incoming.Decision)
}

func TestProviderUsesChallengeAndSendsText(t *testing.T) {
	var received bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant", "expire": 3600})
		case "/open-apis/im/v1/messages":
			received = true
			require.Equal(t, "Bearer tenant", r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	provider := feishu.NewProvider(config.FeishuConfig{AppID: "app", AppSecret: "secret", APIBaseURL: server.URL})
	body := []byte(`{"challenge":"challenge-1","token":"","header":{"token":""}}`)

	incoming, err := provider.Decode(context.Background(), messaging.Request{Body: body})
	require.NoError(t, err)
	require.Equal(t, "challenge-1", incoming.Challenge)
	require.NoError(t, provider.Send(context.Background(), messaging.Notification{Destination: "oc-chat", Text: "hello"}))
	require.True(t, received)
}

func TestProviderRedactsNonSuccessResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "tenant_access_token": "tenant", "expire": 3600})
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("private provider response"))
	}))
	defer server.Close()
	provider := feishu.NewProvider(config.FeishuConfig{AppID: "app", AppSecret: "secret", APIBaseURL: server.URL})

	err := provider.Send(context.Background(), messaging.Notification{Destination: "oc-chat", Text: "hello"})

	require.EqualError(t, err, "feishu send returned 403 Forbidden")
	require.NotContains(t, err.Error(), "private provider response")
}
