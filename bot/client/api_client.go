package client

import (
	"context"
	"net/url"
)

type APIClient struct {
	*Client
}

func NewAPIClient() *APIClient {
	return &APIClient{
		Client: NewClient("https://polymarket.com/api"),
	}
}

func (c *APIClient) GetGeoBlock(ctx context.Context) (*GeoBlockResponse, error) {
	endpoint := "/geoblock"
	params := url.Values{}

	response := &GeoBlockResponse{}
	if err := c.Client.get(ctx, endpoint, params, response); err != nil {
		return nil, err
	}
	return response, nil
}
