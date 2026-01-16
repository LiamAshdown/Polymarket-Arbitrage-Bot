package client

import (
	"context"
	"net/url"
)

type GammaClient struct {
	*Client
}

func NewGammaClient() *GammaClient {
	return &GammaClient{
		Client: NewClient("https://gamma-api.polymarket.com"),
	}
}

func (c *GammaClient) GetEvents(ctx context.Context, tagId string) (MarketResponse, error) {
	endpoint := "/events"
	params := url.Values{}
	params.Set("tag_id", tagId) // 102467
	params.Set("closed", "false")
	params.Set("limit", "10000")

	response := MarketResponse{}
	if err := c.Client.get(ctx, endpoint, params, &response); err != nil {
		return nil, err
	}
	return response, nil
}
