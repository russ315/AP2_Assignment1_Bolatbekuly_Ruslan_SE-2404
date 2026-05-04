package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"ap2/order-service/internal/usecase"
)

type RestClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewRestClient(baseURL string, httpClient *http.Client) *RestClient {
	return &RestClient{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		httpClient: httpClient,
	}
}

type paymentAPIResponse struct {
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
}

func (c *RestClient) Authorize(ctx context.Context, orderID string, amount int64, customerEmail string) (string, string, error) {
	payload := map[string]any{
		"order_id":        orderID,
		"amount":          amount,
		"customer_email":  customerEmail,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/payments", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if isUnavailable(err) {
			return "", "", usecase.ErrPaymentUnavailable
		}
		return "", "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusOK:
		var pr paymentAPIResponse
		if err := json.Unmarshal(b, &pr); err != nil {
			return "", "", fmt.Errorf("decode payment response: %w", err)
		}
		return pr.TransactionID, pr.Status, nil
	case http.StatusConflict:
		return "", "", usecase.ErrPaymentAlreadyRecorded
	case http.StatusBadRequest:
		return "", "", usecase.ErrPaymentInvalidArgument
	default:
		if resp.StatusCode >= http.StatusInternalServerError {
			return "", "", usecase.ErrPaymentUnavailable
		}
		return "", "", fmt.Errorf("payment service returned %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
}

// GetStatus implements usecase.PaymentAuthorizer.
func (c *RestClient) GetStatus(ctx context.Context, orderID string) (string, error) {
	u := fmt.Sprintf("%s/payments/%s", c.baseURL, url.PathEscape(strings.TrimSpace(orderID)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if isUnavailable(err) {
			return "", usecase.ErrPaymentUnavailable
		}
		return "", err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	switch resp.StatusCode {
	case http.StatusOK:
		var pr struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(b, &pr); err != nil {
			return "", fmt.Errorf("decode payment get: %w", err)
		}
		return pr.Status, nil
	case http.StatusNotFound:
		return "", usecase.ErrPaymentNotFound
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return "", usecase.ErrPaymentUnavailable
	default:
		return "", fmt.Errorf("payment service returned %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
}

func isUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && (ne.Timeout() || strings.Contains(strings.ToLower(err.Error()), "connection refused")) {
		return true
	}
	return false
}
