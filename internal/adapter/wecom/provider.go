package wecom

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"chatops-deploy/internal/config"
	"chatops-deploy/internal/domain"
	"chatops-deploy/internal/messaging"
)

type Provider struct {
	cfg        config.WeComConfig
	http       *http.Client
	configured bool
	mu         sync.Mutex
	token      string
	expires    time.Time
}

func NewProvider(cfg config.WeComConfig) *Provider {
	configured := strings.TrimSpace(cfg.CorpID) != "" && strings.TrimSpace(cfg.AgentID) != "" && strings.TrimSpace(cfg.Secret) != "" && strings.TrimSpace(cfg.Token) != "" && strings.TrimSpace(cfg.APIBaseURL) != ""
	return &Provider{cfg: cfg, http: &http.Client{Timeout: 10 * time.Second}, configured: configured}
}

func (p *Provider) Name() domain.MessageProvider { return domain.MessageProviderWeCom }
func (p *Provider) Capabilities(context.Context) messaging.Capabilities {
	result := messaging.Capabilities{Provider: p.Name(), Configured: p.configured}
	if !p.configured {
		result.Reason = "WeCom credentials are not configured"
	}
	return result
}
func (p *Provider) Check(ctx context.Context) error {
	if !p.configured {
		return errors.New("wecom provider is not configured")
	}
	_, err := p.accessToken(ctx)
	return err
}

func (p *Provider) Decode(_ context.Context, request messaging.Request) (messaging.Incoming, error) {
	if !p.configured {
		return messaging.Incoming{}, errors.New("wecom provider is not configured")
	}
	signature := request.Query["msg_signature"]
	if signature == "" {
		signature = request.Headers["X-WeCom-Signature"]
	}
	if signature == "" || signature != Signature(p.cfg.Token, request.Query["timestamp"], request.Query["nonce"], string(request.Body)) {
		return messaging.Incoming{}, errors.New("invalid wecom signature")
	}
	var payload struct {
		EventID        string   `json:"event_id"`
		ConversationID string   `json:"conversation_id"`
		ChatID         string   `json:"chat_id"`
		SubjectID      string   `json:"subject_id"`
		UserID         string   `json:"userid"`
		DisplayName    string   `json:"display_name"`
		Text           string   `json:"text"`
		Content        string   `json:"content"`
		Command        string   `json:"command"`
		Arguments      []string `json:"arguments"`
		OperationID    string   `json:"operation_id"`
		Decision       string   `json:"decision"`
		Challenge      string   `json:"challenge"`
	}
	if err := json.Unmarshal(request.Body, &payload); err != nil {
		return messaging.Incoming{}, errors.New("invalid wecom event")
	}
	if payload.ConversationID == "" {
		payload.ConversationID = payload.ChatID
	}
	if payload.SubjectID == "" {
		payload.SubjectID = payload.UserID
	}
	if payload.SubjectID != "" {
		payload.ConversationID = payload.SubjectID
	}
	if payload.Text == "" {
		payload.Text = payload.Content
	}
	command, args := parseCommand(payload.Command, payload.Arguments, payload.Text)
	return messaging.Incoming{Provider: p.Name(), EventID: payload.EventID, ConversationID: payload.ConversationID, SubjectID: payload.SubjectID, DisplayName: payload.DisplayName, Command: command, Arguments: args, OperationID: payload.OperationID, Decision: payload.Decision, Challenge: payload.Challenge}, nil
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
	endpoint := strings.TrimRight(p.cfg.APIBaseURL, "/") + "/cgi-bin/gettoken?" + url.Values{"corpid": {p.cfg.CorpID}, "corpsecret": {p.cfg.Secret}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if resp.StatusCode/100 != 2 || result.ErrCode != 0 || result.AccessToken == "" {
		return "", errors.New("wecom access token request failed")
	}
	p.token, p.expires = result.AccessToken, time.Now().Add(time.Duration(result.ExpiresIn)*time.Second)
	return p.token, nil
}

func (p *Provider) Send(ctx context.Context, notification messaging.Notification) error {
	if !p.configured {
		return errors.New("wecom provider is not configured")
	}
	token, err := p.accessToken(ctx)
	if err != nil {
		return err
	}
	text := notification.Text
	if notification.ApprovalCard != nil && notification.OperationID != "" {
		text = fmt.Sprintf("%s\n批准：/approve %s approved\n拒绝：/approve %s rejected", notification.Text, notification.OperationID, notification.OperationID)
	}
	body, err := json.Marshal(map[string]any{"touser": notification.Destination, "msgtype": "text", "agentid": p.cfg.AgentID, "text": map[string]string{"content": text}})
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(p.cfg.APIBaseURL, "/") + "/cgi-bin/message/send?access_token=" + url.QueryEscape(token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("wecom send returned %s", resp.Status)
	}
	var result struct {
		ErrCode int `json:"errcode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.ErrCode != 0 {
		return errors.New("wecom send failed")
	}
	return nil
}

func Signature(token, timestamp, nonce, body string) string {
	values := []string{token, timestamp, nonce, body}
	sort.Strings(values)
	hash := sha1.Sum([]byte(strings.Join(values, "")))
	return hex.EncodeToString(hash[:])
}
