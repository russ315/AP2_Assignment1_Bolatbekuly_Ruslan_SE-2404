package email

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// MailjetSender sends email via Mailjet REST API v3.1.
type MailjetSender struct {
	apiKey    string
	apiSecret string
	fromEmail string
	fromName  string
	client    *http.Client
}

func NewMailjet(apiKey, apiSecret, fromEmail, fromName string) *MailjetSender {
	return &MailjetSender{
		apiKey:    strings.TrimSpace(apiKey),
		apiSecret: strings.TrimSpace(apiSecret),
		fromEmail: strings.TrimSpace(fromEmail),
		fromName:  strings.TrimSpace(fromName),
		client: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

type mailjetSendRequest struct {
	Messages []mailjetMessage `json:"Messages"`
}

type mailjetMessage struct {
	From     mailjetAddr   `json:"From"`
	To       []mailjetAddr `json:"To"`
	Subject  string        `json:"Subject"`
	TextPart string        `json:"TextPart"`
}

type mailjetAddr struct {
	Email string `json:"Email"`
	Name  string `json:"Name"`
}

func (m *MailjetSender) SendPaymentReceipt(ctx context.Context, in SendInput) error {
	fmt.Println("Sending payment receipt to", in.ToEmail)
	if m.apiKey == "" || m.apiSecret == "" || m.fromEmail == "" {
		return fmt.Errorf("mailjet: missing api key, secret, or from email")
	}
	to := strings.TrimSpace(in.ToEmail)
	if to == "" {
		return fmt.Errorf("mailjet: empty recipient")
	}
	fromName := m.fromName
	if fromName == "" {
		fromName = "Notifications"
	}
	bodyText := fmt.Sprintf(
		"Payment receipt\nPayment ID: %s\nOrder: %s\nAmount (cents): %d\nStatus: %s\n",
		in.PaymentID, in.OrderID, in.AmountCents, in.Status,
	)
	payload := mailjetSendRequest{
		Messages: []mailjetMessage{
			{
				From:     mailjetAddr{Email: m.fromEmail, Name: fromName},
				To:       []mailjetAddr{{Email: to, Name: to}},
				Subject:  fmt.Sprintf("Payment confirmed — order %s", in.OrderID),
				TextPart: bodyText,
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("mailjet marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.mailjet.com/v3.1/send", bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("mailjet request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	auth := base64.StdEncoding.EncodeToString([]byte(m.apiKey + ":" + m.apiSecret))
	req.Header.Set("Authorization", "Basic "+auth)

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("mailjet http: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mailjet status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}
