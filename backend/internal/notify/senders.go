package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// OutMessage — сообщение для отправки через внешний канал.
type OutMessage struct {
	QueueID          int64
	ToName           string
	ToEmail          string
	ToPhone          string
	ToTelegram       string
	ToPush           string
	Subject          string
	Body             string
	NotificationType string
}

// Sender — отправитель одного канала.
type Sender interface {
	Send(ctx context.Context, creds map[string]string, m OutMessage) (string, error)
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// ---------------------------------------------------------------- SMTPSender

// SMTPSender — настоящая отправка email через SMTP (PLAIN auth, STARTTLS/465).
// creds: host*, port*, username, password, from*, tls ("true"/"false", default auto).
type SMTPSender struct{}

func (SMTPSender) Send(ctx context.Context, creds map[string]string, m OutMessage) (string, error) {
	host, port := creds["host"], creds["port"]
	from := creds["from"]
	if host == "" || port == "" || from == "" {
		return "", fmt.Errorf("smtp: host/port/from required")
	}
	if m.ToEmail == "" {
		return "", fmt.Errorf("smtp: recipient email empty")
	}
	msgID := fmt.Sprintf("<%d.%d@rms.local>", m.QueueID, time.Now().UnixNano())
	subj := mime.BEncoding.Encode("utf-8", ifEmpty(m.Subject, m.NotificationType))
	body := "From: " + from + "\r\n" +
		"To: " + m.ToEmail + "\r\n" +
		"Subject: " + subj + "\r\n" +
		"Message-ID: " + msgID + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: 8bit\r\n\r\n" +
		m.Body + "\r\n"

	addr := net.JoinHostPort(host, port)
	useTLS := strings.ToLower(creds["tls"])
	var c *smtp.Client
	var err error
	if port == "465" || useTLS == "implicit" {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, &tls.Config{ServerName: host})
		if err != nil {
			return "", fmt.Errorf("smtp dial: %w", err)
		}
		c, err = smtp.NewClient(conn, host)
		if err != nil {
			return "", fmt.Errorf("smtp client: %w", err)
		}
	} else {
		c, err = smtp.Dial(addr)
		if err != nil {
			return "", fmt.Errorf("smtp dial: %w", err)
		}
	}
	defer c.Close()
	_ = ctx
	if err := c.Hello("rms.local"); err != nil {
		return "", fmt.Errorf("smtp hello: %w", err)
	}
	if useTLS != "false" {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err := c.StartTLS(&tls.Config{ServerName: host}); err != nil {
				return "", fmt.Errorf("smtp starttls: %w", err)
			}
		}
	}
	if creds["username"] != "" {
		auth := smtp.PlainAuth("", creds["username"], creds["password"], host)
		if err := c.Auth(auth); err != nil {
			return "", fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return "", fmt.Errorf("smtp mail: %w", err)
	}
	if err := c.Rcpt(m.ToEmail); err != nil {
		return "", fmt.Errorf("smtp rcpt: %w", err)
	}
	w, err := c.Data()
	if err != nil {
		return "", fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		return "", fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("smtp close: %w", err)
	}
	_ = c.Quit()
	return msgID, nil
}

// -------------------------------------------------------------- TelegramSender

// TelegramSender — настоящая отправка через Bot API.
// creds: bot_token*, api_base (default https://api.telegram.org).
// Адресат: chat_id получателя.
type TelegramSender struct{}

func (TelegramSender) Send(ctx context.Context, creds map[string]string, m OutMessage) (string, error) {
	token := creds["bot_token"]
	if token == "" {
		return "", fmt.Errorf("telegram: bot_token required")
	}
	if m.ToTelegram == "" {
		return "", fmt.Errorf("telegram: recipient chat id empty")
	}
	base := creds["api_base"]
	if base == "" {
		base = "https://api.telegram.org"
	}
	text := m.Body
	if m.Subject != "" {
		text = m.Subject + "\n" + m.Body
	}
	payload, _ := json.Marshal(map[string]string{"chat_id": m.ToTelegram, "text": text})
	req, err := http.NewRequestWithContext(ctx, "POST", base+"/bot"+token+"/sendMessage", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("telegram: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
		Description string `json:"description"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if !out.OK {
		return "", fmt.Errorf("telegram: %s", firstNonEmpty(out.Description, resp.Status))
	}
	return fmt.Sprintf("tg-%d", out.Result.MessageID), nil
}

// ------------------------------------------------------------ GenericHTTPSender

// GenericHTTPSender — универсальный POST JSON {to,text} для SMS/PUSH/WhatsApp
// на URL из настроек (работает с шлюзами, принимающими такой формат).
// creds: api_url*, api_key (Bearer), sender, field_to/field_text (имена полей).
type GenericHTTPSender struct {
	DefaultToField   string
	DefaultTextField string
}

func (g GenericHTTPSender) Send(ctx context.Context, creds map[string]string, m OutMessage) (string, error) {
	url := creds["api_url"]
	if url == "" {
		return "", fmt.Errorf("http gateway: api_url required")
	}
	to := m.ToPhone
	if g.DefaultToField == "token" {
		to = m.ToPush
	}
	if to == "" {
		return "", fmt.Errorf("http gateway: recipient address empty")
	}
	toField := creds["field_to"]
	if toField == "" {
		toField = g.DefaultToField
	}
	textField := creds["field_text"]
	if textField == "" {
		textField = g.DefaultTextField
	}
	payload, _ := json.Marshal(map[string]interface{}{toField: to, textField: m.Body})
	if s := creds["sender"]; s != "" {
		var extra map[string]interface{}
		_ = json.Unmarshal(payload, &extra)
		extra["sender"] = s
		payload, _ = json.Marshal(extra)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if creds["api_key"] != "" {
		req.Header.Set("Authorization", "Bearer "+creds["api_key"])
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http gateway: %w", err)
	}
	defer resp.Body.Close()
	var out map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("http gateway: status %s", resp.Status)
	}
	for _, k := range []string{"id", "message_id", "sms_id"} {
		if v, ok := out[k]; ok {
			return fmt.Sprintf("%v", v), nil
		}
	}
	if res, ok := out["result"].(map[string]interface{}); ok {
		for _, k := range []string{"message_id", "id"} {
			if v, ok := res[k]; ok {
				return fmt.Sprintf("%v", v), nil
			}
		}
	}
	return fmt.Sprintf("http-%d", m.QueueID), nil
}

func ifEmpty(s, fallback string) string {
	if s != "" {
		return s
	}
	return fallback
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
