package feishu

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"chatops-deploy/internal/config"
	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/messaging"
)

type Provider struct {
	client            *Client
	verificationToken string
	encryptKey        string
	configured        bool
}

func NewProvider(cfg config.FeishuConfig) *Provider {
	configured := strings.TrimSpace(cfg.AppID) != "" && strings.TrimSpace(cfg.AppSecret) != "" && strings.TrimSpace(cfg.APIBaseURL) != ""
	return &Provider{client: NewClient(cfg.AppID, cfg.AppSecret, cfg.APIBaseURL), verificationToken: cfg.VerificationToken, encryptKey: cfg.EncryptKey, configured: configured}
}

func (p *Provider) Name() domain.MessageProvider { return domain.MessageProviderFeishu }

func (p *Provider) Capabilities(context.Context) messaging.Capabilities {
	capabilities := messaging.Capabilities{Provider: p.Name(), Configured: p.configured, Healthy: p.configured}
	if !p.configured {
		capabilities.Reason = "Feishu app credentials are not configured"
	}
	return capabilities
}

func (p *Provider) Check(context.Context) error {
	if !p.configured {
		return errors.New("feishu provider is not configured")
	}
	return nil
}

func (p *Provider) Decode(_ context.Context, request messaging.Request) (messaging.Incoming, error) {
	if !p.configured {
		return messaging.Incoming{}, errors.New("feishu provider is not configured")
	}
	timestamp := request.Headers["X-Lark-Request-Timestamp"]
	if timestamp == "" {
		timestamp = request.Headers["X-Request-Timestamp"]
	}
	nonce := request.Headers["X-Lark-Request-Nonce"]
	if nonce == "" {
		nonce = request.Headers["X-Request-Nonce"]
	}
	signature := request.Headers["X-Lark-Signature"]
	if signature == "" {
		signature = request.Headers["X-Request-Signature"]
	}
	if !VerifySignature(timestamp, nonce, p.encryptKey, signature, request.Body) {
		return messaging.Incoming{}, errors.New("invalid feishu signature")
	}
	plain, err := DecryptEvent(request.Body, p.encryptKey)
	if err != nil {
		return messaging.Incoming{}, fmt.Errorf("decode feishu event: %w", err)
	}
	event, err := DecodeEvent(plain, p.verificationToken)
	if err != nil {
		return messaging.Incoming{}, err
	}
	incoming := messaging.Incoming{Provider: p.Name(), EventID: event.Header.EventID, ConversationID: event.Event.Message.ChatID, SubjectID: event.Event.Sender.SenderID.OpenID, Challenge: event.Challenge}
	if incoming.SubjectID == "" {
		incoming.SubjectID = event.Event.Operator.OpenID
	}
	incoming.Command, incoming.Arguments = Command(event.Event.Message.Content)
	if event.Event.Action.Value != nil {
		incoming.OperationID = event.Event.Action.Value["operation_id"]
		incoming.Decision = event.Event.Action.Value["decision"]
	}
	return incoming, nil
}

func (p *Provider) Send(ctx context.Context, notification messaging.Notification) error {
	if !p.configured {
		return errors.New("feishu provider is not configured")
	}
	if notification.ApprovalCard != nil {
		return p.client.SendCard(ctx, notification.Destination, notification.ApprovalCard)
	}
	return p.client.SendText(ctx, notification.Destination, notification.Text)
}
