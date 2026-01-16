package client

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

type ClobClient struct {
	*Client
}

func NewClobClient() *ClobClient {
	return &ClobClient{
		Client: NewClient("https://clob.polymarket.com"),
	}
}

func (c *ClobClient) GetFeeRateBps(ctx context.Context, tokenId string) (int, error) {
	endpoint := "/fee-rate"
	params := url.Values{}
	params.Set("token_id", tokenId)

	var resp FeeRateResp
	if err := c.Client.get(ctx, endpoint, params, &resp); err != nil {
		return 0, err
	}
	return resp.FeeRateBps, nil
}

func (c *ClobClient) GetPrice(ctx context.Context, tokenId string, side string) (float64, error) {
	endpoint := "/price"
	params := url.Values{}
	params.Set("token_id", tokenId)
	params.Set("side", side)

	response := GetPriceResponse{}
	if err := c.Client.get(ctx, endpoint, params, &response); err != nil {
		return 0, err
	}

	price, err := strconv.ParseFloat(response.Price, 64)
	if err != nil {
		return 0, err
	}
	return price, nil
}

func (c *ClobClient) GetBook(ctx context.Context, tokenId string) (*GetBookResponse, error) {
	endpoint := "/book"
	params := url.Values{}
	params.Set("token_id", tokenId)

	response := &GetBookResponse{}
	if err := c.Client.get(ctx, endpoint, params, response); err != nil {
		return nil, err
	}
	return response, nil
}

func (c *ClobClient) CreateOrDeriveApiKey(ctx context.Context, privateKeyHex string) (*ApiKeyResponse, error) {
	signer, err := NewEIP712OrderSigner(privateKeyHex, 137, "0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E")
	if err != nil {
		return nil, err
	}

	address := signer.GetAddress().Hex()

	timestamp := time.Now().Unix()
	nonce := 0

	signature, err := CreateL1AuthSignature(signer, timestamp, nonce)
	if err != nil {
		return nil, fmt.Errorf("failed to create L1 signature: %w", err)
	}

	endpoint := "/auth/derive-api-key"

	headers := map[string]string{
		"POLY_ADDRESS":   address,
		"POLY_SIGNATURE": signature,
		"POLY_TIMESTAMP": strconv.FormatInt(timestamp, 10),
		"POLY_NONCE":     strconv.Itoa(nonce),
	}

	response := &ApiKeyResponse{}
	if err := c.Client.getWithHeaders(ctx, endpoint, headers, response); err != nil {
		return nil, err
	}

	return response, nil
}

func (c *ClobClient) GetAverageLatency(ctx context.Context) (time.Duration, error) {
	endpoint := "/sampling-markets"
	params := url.Values{}
	numRequests := 5

	var totalLatency time.Duration
	successfulRequests := 0

	for i := 0; i < numRequests; i++ {
		start := time.Now()

		var resp interface{}
		if err := c.Client.get(ctx, endpoint, params, &resp); err != nil {
			continue
		}

		latency := time.Since(start)
		totalLatency += latency
		successfulRequests++

		if i < numRequests-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	if successfulRequests == 0 {
		return 0, fmt.Errorf("all latency test requests failed")
	}

	return totalLatency / time.Duration(successfulRequests), nil
}
