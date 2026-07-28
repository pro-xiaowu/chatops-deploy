package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	appID, appSecret, baseURL string
	http                      *http.Client
	mu                        sync.Mutex
	token                     string
	expires                   time.Time
}

func NewClient(appID, secret, baseURL string) *Client {
	return &Client{appID: appID, appSecret: secret, baseURL: baseURL, http: &http.Client{Timeout: 10 * time.Second}}
}

func (c *Client) Configured() bool {
	return c != nil && strings.TrimSpace(c.appID) != "" && strings.TrimSpace(c.appSecret) != "" && strings.TrimSpace(c.baseURL) != ""
}
func (c *Client) tenantToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expires.Add(-time.Minute)) {
		return c.token, nil
	}
	body, _ := json.Marshal(map[string]string{"app_id": c.appID, "app_secret": c.appSecret})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/open-apis/auth/v3/tenant_access_token/internal", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Code   int    `json:"code"`
		Msg    string `json:"msg"`
		Token  string `json:"tenant_access_token"`
		Expire int    `json:"expire"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Code != 0 {
		return "", fmt.Errorf("feishu auth: %s", out.Msg)
	}
	c.token = out.Token
	c.expires = time.Now().Add(time.Duration(out.Expire) * time.Second)
	return c.token, nil
}
func (c *Client) SendText(ctx context.Context, chatID, text string) error {
	token, err := c.tenantToken(ctx)
	if err != nil {
		return err
	}
	content, _ := json.Marshal(map[string]string{"text": text})
	body, _ := json.Marshal(map[string]any{"receive_id": chatID, "msg_type": "text", "content": string(content)})
	endpoint := c.baseURL + "/open-apis/im/v1/messages?receive_id_type=chat_id"
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("feishu send returned %s", resp.Status)
	}
	return nil
}
func (c *Client) SendCard(ctx context.Context, chatID string, card map[string]any) error {
	token, err := c.tenantToken(ctx)
	if err != nil {
		return err
	}
	content, _ := json.Marshal(card)
	body, _ := json.Marshal(map[string]any{"receive_id": chatID, "msg_type": "interactive", "content": string(content)})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/open-apis/im/v1/messages?receive_id_type=chat_id", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("feishu card returned %s", resp.Status)
	}
	return nil
}
func ApprovalCard(operationID, kind, target string) map[string]any {
	return map[string]any{"config": map[string]any{"wide_screen_mode": true}, "header": map[string]any{"template": "orange", "title": map[string]string{"tag": "plain_text", "content": "生产变更待审批"}}, "elements": []any{map[string]any{"tag": "markdown", "content": fmt.Sprintf("**操作**：%s\n**目标**：%s\n**操作单**：%s", kind, target, operationID)}, map[string]any{"tag": "action", "actions": []any{map[string]any{"tag": "button", "text": map[string]string{"tag": "plain_text", "content": "批准"}, "type": "primary", "value": map[string]string{"operation_id": operationID, "decision": "approved"}}, map[string]any{"tag": "button", "text": map[string]string{"tag": "plain_text", "content": "拒绝"}, "type": "danger", "value": map[string]string{"operation_id": operationID, "decision": "rejected"}}}}}}
}
func (c *Client) OAuthURL(redirect, state string) string {
	q := url.Values{"app_id": {c.appID}, "redirect_uri": {redirect}, "state": {state}}
	return c.baseURL + "/open-apis/authen/v1/authorize?" + q.Encode()
}
func (c *Client) ExchangeCode(ctx context.Context, code, redirect string) (string, error) {
	token, err := c.tenantToken(ctx)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{"grant_type": "authorization_code", "code": code, "redirect_uri": redirect})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/open-apis/authen/v1/oidc/access_token", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID string `json:"open_id"`
		} `json:"data"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Code != 0 || out.Data.OpenID == "" {
		return "", fmt.Errorf("feishu oauth: %s", out.Msg)
	}
	return out.Data.OpenID, nil
}
