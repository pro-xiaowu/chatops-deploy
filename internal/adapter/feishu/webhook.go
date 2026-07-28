package feishu

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

func VerifySignature(timestamp, nonce, encryptKey, signature string, body []byte) bool {
	if timestamp == "" || nonce == "" || encryptKey == "" || signature == "" {
		return false
	}
	sum := sha256.Sum256(append([]byte(timestamp+nonce+encryptKey), body...))
	expected := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(signature)), []byte(expected)) == 1
}

func DecryptEvent(body []byte, encryptKey string) ([]byte, error) {
	if encryptKey == "" {
		return body, nil
	}
	var envelope struct {
		Encrypt string `json:"encrypt"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.Encrypt == "" {
		return body, nil
	}
	encrypted, err := base64.StdEncoding.DecodeString(envelope.Encrypt)
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	if len(encrypted) == 0 || len(encrypted)%aes.BlockSize != 0 {
		return nil, errors.New("invalid encrypted event length")
	}
	plain := make([]byte, len(encrypted))
	cipher.NewCBCDecrypter(block, key[:aes.BlockSize]).CryptBlocks(plain, encrypted)
	padding := int(plain[len(plain)-1])
	if padding <= 0 || padding > aes.BlockSize || padding > len(plain) {
		return nil, errors.New("invalid encrypted event padding")
	}
	return plain[:len(plain)-padding], nil
}

type Event struct {
	Schema    string `json:"schema"`
	Challenge string `json:"challenge"`
	Token     string `json:"token"`
	Header    struct {
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
		Token     string `json:"token"`
	} `json:"header"`
	Event struct {
		Sender struct {
			SenderID struct {
				OpenID string `json:"open_id"`
			} `json:"sender_id"`
		} `json:"sender"`
		Message struct {
			ChatID  string `json:"chat_id"`
			Content string `json:"content"`
		} `json:"message"`
		Action struct {
			Value map[string]string `json:"value"`
		} `json:"action"`
		Operator struct {
			OpenID string `json:"open_id"`
		} `json:"operator"`
	} `json:"event"`
}

func DecodeEvent(body []byte, verificationToken string) (Event, error) {
	var e Event
	if err := json.Unmarshal(body, &e); err != nil {
		return e, err
	}
	token := e.Token
	if token == "" {
		token = e.Header.Token
	}
	if verificationToken != "" && token != verificationToken {
		return e, errors.New("invalid verification token")
	}
	return e, nil
}
func Command(content string) (string, []string) {
	var parsed struct {
		Text string `json:"text"`
	}
	if json.Unmarshal([]byte(content), &parsed) != nil {
		parsed.Text = content
	}
	fields := strings.Fields(strings.TrimSpace(parsed.Text))
	if len(fields) == 0 {
		return "", nil
	}
	return strings.TrimPrefix(strings.ToLower(fields[0]), "/"), fields[1:]
}
func HelpText() string {
	return "可用命令：/deploy <environment-id> <image>、/rollback <environment-id> <revision>、/approve <operation-id> <approved|rejected>、/status <environment-id>、/help"
}
