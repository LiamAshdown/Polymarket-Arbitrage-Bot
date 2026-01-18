package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// PolymarketTradeClient handles authenticated Polymarket CLOB order operations.
type PolymarketTradeClient struct {
	*Client
	auth *PolymarketL2Auth
}

func NewPolymarketTradeClient(auth *PolymarketL2Auth) *PolymarketTradeClient {
	c := NewClient("https://clob.polymarket.com")
	c.SetAuth(auth)
	return &PolymarketTradeClient{Client: c, auth: auth}
}

// PolymarketOrder represents the signed order object
type PolymarketOrder struct {
	Salt          uint64 `json:"salt"`
	Maker         string `json:"maker"`
	Signer        string `json:"signer"`
	Taker         string `json:"taker"`
	TokenID       string `json:"tokenId"`
	MakerAmount   string `json:"makerAmount"`
	TakerAmount   string `json:"takerAmount"`
	Expiration    string `json:"expiration"`
	Nonce         string `json:"nonce"`
	FeeRateBps    string `json:"feeRateBps"`
	Side          string `json:"side"` // "BUY" or "SELL"
	SignatureType int    `json:"signatureType"`
	Signature     string `json:"signature"`
}

// PolymarketOrderRequest is the top-level order placement payload
type PolymarketOrderRequest struct {
	Order     PolymarketOrder `json:"order"`
	Owner     string          `json:"owner"`     // API key UUID
	OrderType string          `json:"orderType"` // "FOK", "GTC", "GTD"
	PostOnly  bool            `json:"postOnly,omitempty"`
}

type PolymarketOrderResponse struct {
	Success     bool        `json:"success"`
	ErrorMsg    string      `json:"errorMsg"`
	OrderID     string      `json:"orderId"`
	OrderHashes []string    `json:"orderHashes"`
	Status      OrderStatus `json:"status"`
}

type PolymarketBatchOrderRequest struct {
	Orders []PolymarketOrderRequest `json:"orders"`
}

type PolymarketBatchOrderResponse struct {
	Responses []PolymarketOrderResponse `json:"responses"`
}

func (tc *PolymarketTradeClient) PlaceOrder(ctx context.Context, req PolymarketOrderRequest) (*PolymarketOrderResponse, error) {
	if tc.auth == nil {
		return nil, errors.New("auth required: missing Polymarket L2 credentials")
	}

	endpoint := "/order"

	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "")
	if err := encoder.Encode(req); err != nil {
		return nil, err
	}

	bodyBytes := bytes.TrimSpace(buf.Bytes())
	bodyStr := string(bodyBytes)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tc.baseUrl+endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if err := tc.auth.SignWithBody(httpReq, bodyStr); err != nil {
		return nil, err
	}

	resp, err := tc.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, errBody.String())
	}

	var result PolymarketOrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Success {
		return &result, fmt.Errorf("order placement failed: %s", result.ErrorMsg)
	}

	return &result, nil
}

func (tc *PolymarketTradeClient) PlaceBatchOrders(ctx context.Context, orders []PolymarketOrderRequest) ([]PolymarketOrderResponse, error) {
	if tc.auth == nil {
		return nil, errors.New("auth required: missing Polymarket L2 credentials")
	}

	if len(orders) == 0 {
		return nil, errors.New("no orders provided")
	}

	if len(orders) > 15 {
		return nil, errors.New("maximum 15 orders per batch")
	}

	endpoint := "/orders"

	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "")
	if err := encoder.Encode(orders); err != nil {
		return nil, err
	}

	bodyBytes := bytes.TrimSpace(buf.Bytes())
	bodyStr := string(bodyBytes)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tc.baseUrl+endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if err := tc.auth.SignWithBody(httpReq, bodyStr); err != nil {
		return nil, err
	}

	resp, err := tc.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errBody bytes.Buffer
		errBody.ReadFrom(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, errBody.String())
	}

	var results []PolymarketOrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	return results, nil
}

type PolymarketCancelResponse struct {
	Success  bool   `json:"success"`
	ErrorMsg string `json:"errorMsg"`
	Status   string `json:"status"`
}

type PolymarketBalanceResponse struct {
	Balance   string `json:"balance"`
	Allowance string `json:"allowance"`
}

func (tc *PolymarketTradeClient) GetBalance(ctx context.Context) (float64, error) {
	if tc.auth == nil {
		return 0, errors.New("auth required: missing Polymarket L2 credentials")
	}

	params := url.Values{}
	params.Set("asset_type", "COLLATERAL")

	var response PolymarketBalanceResponse
	if err := tc.get(ctx, "/balance-allowance", params, &response); err != nil {
		return 0, fmt.Errorf("balance request failed: %w", err)
	}

	var balance float64
	if _, err := fmt.Sscanf(response.Balance, "%f", &balance); err != nil {
		return 0, fmt.Errorf("failed to parse balance: %w", err)
	}

	return balance, nil
}

func (tc *PolymarketTradeClient) CancelOrder(ctx context.Context, orderID string) (*PolymarketCancelResponse, error) {
	if tc.auth == nil {
		return nil, errors.New("auth required: missing Polymarket L2 credentials")
	}

	endpoint := fmt.Sprintf("/order/%s", orderID)

	var result PolymarketCancelResponse
	if err := tc.delete(ctx, endpoint, &result); err != nil {
		return nil, err
	}

	if !result.Success {
		return &result, fmt.Errorf("cancel failed: %s", result.ErrorMsg)
	}

	return &result, nil
}
