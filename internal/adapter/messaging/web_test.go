package messaging_test

import (
	"context"
	"testing"

	messagingweb "chatops-deploy/internal/adapter/messaging"
	"chatops-deploy/internal/messaging"
	"github.com/stretchr/testify/require"
)

func TestWebProviderDoesNotDecodeOrSendExternalMessages(t *testing.T) {
	provider := messagingweb.New()

	require.True(t, provider.Capabilities(context.Background()).Configured)
	_, err := provider.Decode(context.Background(), messaging.Request{})
	require.ErrorIs(t, err, messaging.ErrUnsupportedDecode)
	require.EqualError(t, provider.Send(context.Background(), messaging.Notification{Text: "ignored"}), "web provider does not send messages")
	require.NoError(t, provider.Check(context.Background()))
}
