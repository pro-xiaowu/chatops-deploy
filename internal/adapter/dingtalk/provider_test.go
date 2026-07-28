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
	query := map[string]string{"timestamp": "100"}
	query["signature"] = dingtalk.Signature("token", query["timestamp"], string(body))

	incoming, err := provider.Decode(context.Background(), messaging.Request{Body: body, Query: query})

	require.NoError(t, err)
	require.Equal(t, "dingtalk", string(incoming.Provider))
	require.Equal(t, "event-1", incoming.EventID)
	require.Equal(t, "user-1", incoming.ConversationID)
	require.Equal(t, "user-1", incoming.SubjectID)
	require.Equal(t, "rollback", incoming.Command)
	require.Equal(t, []string{"env-1", "3"}, incoming.Arguments)
}

func TestProviderRejectsMissingCallbackSignature(t *testing.T) {
	provider := dingtalk.NewProvider(config.DingTalkConfig{ClientID: "client", ClientSecret: "secret", RobotCode: "robot", EventToken: "token", APIBaseURL: "https://example.test"})

	_, err := provider.Decode(context.Background(), messaging.Request{Body: []byte(`{"msgId":"event-1"}`)})

	require.EqualError(t, err, "invalid dingtalk signature")
}

func TestProviderSendsThroughAccessTokenEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/oauth2/accessToken" {
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "access", "expireIn": 3600})
			return
		}
		require.Equal(t, "/v1.0/robot/oToMessages/batchSend", r.URL.Path)
		require.Equal(t, "access", r.Header.Get("x-acs-dingtalk-access-token"))
		var payload struct {
			Message string `json:"msgParam"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		require.Contains(t, payload.Message, "/approve operation-1 approved")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": ""})
	}))
	defer server.Close()
	provider := dingtalk.NewProvider(config.DingTalkConfig{ClientID: "client", ClientSecret: "secret", RobotCode: "robot", EventToken: "token", APIBaseURL: server.URL})

	require.NoError(t, provider.Send(context.Background(), messaging.Notification{OperationID: "operation-1", Destination: "user-1", Text: "approval", ApprovalCard: map[string]any{"kind": "approval"}}))
}

func TestProviderReportsMissingCredentials(t *testing.T) {
	provider := dingtalk.NewProvider(config.DingTalkConfig{})
	require.False(t, provider.Capabilities(context.Background()).Configured)
	require.EqualError(t, provider.Check(context.Background()), "dingtalk provider is not configured")
}

func TestProviderRejectsBusinessErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/oauth2/accessToken" {
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "access", "expireIn": 3600})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Forbidden", "message": "private details"})
	}))
	t.Cleanup(server.Close)
	provider := dingtalk.NewProvider(config.DingTalkConfig{ClientID: "client", ClientSecret: "secret", RobotCode: "robot", EventToken: "token", APIBaseURL: server.URL})

	err := provider.Send(context.Background(), messaging.Notification{Destination: "user-1", Text: "hello"})

	require.EqualError(t, err, "dingtalk send failed")
	require.NotContains(t, err.Error(), "private details")
}
