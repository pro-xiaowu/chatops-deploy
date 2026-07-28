package dingtalk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"chatops-deploy/internal/config"
	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/messaging"
)

type Provider struct {
	cfg        config.DingTalkConfig
	http       *http.Client
	configured bool
	mu         sync.Mutex
	token      string
	expires    time.Time
}

func NewProvider(cfg config.DingTalkConfig) *Provider {
	configured := strings.TrimSpace(cfg.ClientID) != "" && strings.TrimSpace(cfg.ClientSecret) != "" && strings.TrimSpace(cfg.RobotCode) != "" && strings.TrimSpace(cfg.EventToken) != "" && strings.TrimSpace(cfg.APIBaseURL) != ""
	return &Provider{cfg: cfg, http: &http.Client{Timeout: 10 * time.Second}, configured: configured}
}

func (p *Provider) Name() domain.MessageProvider { return domain.MessageProviderDingTalk }
func (p *Provider) Capabilities(context.Context) messaging.Capabilities {
	result := messaging.Capabilities{Provider: p.Name(), Configured: p.configured, Healthy: p.configured}
	if !p.configured {
		result.Reason = "DingTalk credentials are not configured"
	}
	return result
}
func (p *Provider) Check(context.Context) error {
	if !p.configured {
		return errors.New("dingtalk provider is not configured")
	}
	return nil
}

func (p *Provider) Decode(_ context.Context, request messaging.Request) (messaging.Incoming, error) {
	if !p.configured {
		return messaging.Incoming{}, errors.New("dingtalk provider is not configured")
	}
	if token := request.Headers["X-DingTalk-Token"]; token != "" && token != p.cfg.EventToken {
		return messaging.Incoming{}, errors.New("invalid dingtalk token")
	}
	var payload struct {
		MsgID          string `json:"msgId"`
		EventID        string `json:"eventId"`
		ConversationID string `json:"conversationId"`
		ChatID         string `json:"chatId"`
		SenderID       string `json:"senderStaffId"`
		SenderNick     string `json:"senderNick"`
		Text           struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		} `json:"text"`
		Command     string   `json:"command"`
		Arguments   []string `json:"arguments"`
		OperationID string   `json:"operation_id"`
		Decision    string   `json:"decision"`
		Challenge   string   `json:"challenge"`
	}
	if err := json.Unmarshal(request.Body, &payload); err != nil {
		return messaging.Incoming{}, errors.New("invalid dingtalk event")
	}
	if payload.EventID == "" {
		payload.EventID = payload.MsgID
	}
	if payload.ConversationID == "" {
		payload.ConversationID = payload.ChatID
	}
	text := payload.Text.Content
	if text == "" {
		text = payload.Text.Text
	}
	command, args := parseCommand(payload.Command, payload.Arguments, text)
	return messaging.Incoming{Provider: p.Name(), EventID: payload.EventID, ConversationID: payload.ConversationID, SubjectID: payload.SenderID, DisplayName: payload.SenderNick, Command: command, Arguments: args, OperationID: payload.OperationID, Decision: payload.Decision, Challenge: payload.Challenge}, nil
}

func parseCommand(command string, args []string, text string) (string, []string) {
	if command != "" {
		return strings.TrimPrefix(strings.ToLower(command), "/"), args
	}
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return "", nil
	}
	return strings.TrimPrefix(strings.ToLower(fields[0]), "/"), fields[1:]
}

func (p *Provider) accessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Now().Before(p.expires.Add(-time.Minute)) {
		return p.token, nil
	}
	body, err := json.Marshal(map[string]string{"clientId": p.cfg.ClientID, "clientSecret": p.cfg.ClientSecret})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.cfg.APIBaseURL, "/")+"/v1.0/oauth2/accessToken", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int    `json:"expireIn"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if resp.StatusCode/100 != 2 || result.AccessToken == "" {
		return "", errors.New("dingtalk access token request failed")
	}
	p.token, p.expires = result.AccessToken, time.Now().Add(time.Duration(result.ExpireIn)*time.Second)
	return p.token, nil
}

func (p *Provider) Send(ctx context.Context, notification messaging.Notification) error {
	if !p.configured {
		return errors.New("dingtalk provider is not configured")
	}
	token, err := p.accessToken(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"robotCode": p.cfg.RobotCode, "userIds": []string{notification.Destination}, "msgKey": "sampleText", "msgParam": fmt.Sprintf(`{"content":%q}`, notification.Text)})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.cfg.APIBaseURL, "/")+"/v1.0/robot/oToMessages/batchSend", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", token)
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("dingtalk send returned %s", resp.Status)
	}
	return nil
}

func Signature(token, timestamp, body string) string {
	hash := sha256.Sum256([]byte(token + timestamp + body))
	return hex.EncodeToString(hash[:])
}
