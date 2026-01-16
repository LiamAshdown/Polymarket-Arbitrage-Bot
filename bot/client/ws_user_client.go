package client

import (
	"context"
	"encoding/json"
	"fmt"
	"quadbot/logger"
)

type MakerOrder struct {
	AssetID       string        `json:"asset_id"`
	MatchedAmount StringFloat64 `json:"matched_amount"`
	OrderID       string        `json:"order_id"`
	Outcome       string        `json:"outcome"`
	Owner         string        `json:"owner"`
	Price         StringFloat64 `json:"price"`
}

type UserTradeMessage struct {
	AssetID      string        `json:"asset_id"`
	EventType    string        `json:"event_type"`
	ID           string        `json:"id"`
	LastUpdate   string        `json:"last_update"`
	MakerOrders  []MakerOrder  `json:"maker_orders"`
	Market       string        `json:"market"`
	MatchTime    string        `json:"matchtime"`
	Outcome      string        `json:"outcome"`
	Owner        string        `json:"owner"`
	Price        StringFloat64 `json:"price"`
	Side         string        `json:"side"`
	Size         StringFloat64 `json:"size"`
	Status       string        `json:"status"`
	TakerOrderID string        `json:"taker_order_id"`
	Timestamp    string        `json:"timestamp"`
	TradeOwner   string        `json:"trade_owner"`
	Type         string        `json:"type"`
}

type UserOrderMessage struct {
	AssetID         string        `json:"asset_id"`
	AssociateTrades []string      `json:"associate_trades"`
	EventType       string        `json:"event_type"`
	ID              string        `json:"id"`
	Market          string        `json:"market"`
	OrderOwner      string        `json:"order_owner"`
	OriginalSize    StringFloat64 `json:"original_size"`
	Outcome         string        `json:"outcome"`
	Owner           string        `json:"owner"`
	Price           StringFloat64 `json:"price"`
	Side            string        `json:"side"`
	SizeMatched     StringFloat64 `json:"size_matched"`
	Timestamp       string        `json:"timestamp"`
	Type            string        `json:"type"` // PLACEMENT, UPDATE, CANCELLATION
}

type WSUserClient struct {
	*WSClient

	onTrade func(UserTradeMessage)
	onOrder func(UserOrderMessage)

	// Auth credentials for user channel
	apiKey     string
	apiSecret  string
	passphrase string
}

type SubscriptionAuth struct {
	APIKey     string `json:"apiKey"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

type SubscriptionMessage struct {
	Type    string            `json:"type"` // "user" or "market"
	Markets []string          `json:"markets"`
	Auth    *SubscriptionAuth `json:"auth,omitempty"`
}

type WSUserCallbacks struct {
	OnTrade func(UserTradeMessage)
	OnOrder func(UserOrderMessage)
}

func NewWSUserClient(callbacks WSUserCallbacks, apiKey, apiSecret, passphrase string, logger logger.Logger) *WSUserClient {
	base := NewWSClient("wss://ws-subscriptions-clob.polymarket.com/ws/user", logger)
	return &WSUserClient{
		WSClient:   base,
		onTrade:    callbacks.OnTrade,
		onOrder:    callbacks.OnOrder,
		apiKey:     apiKey,
		apiSecret:  apiSecret,
		passphrase: passphrase,
	}
}

func (ws *WSUserClient) OnTrade(cb func(UserTradeMessage)) { ws.onTrade = cb }
func (ws *WSUserClient) OnOrder(cb func(UserOrderMessage)) { ws.onOrder = cb }

func (ws *WSUserClient) SetAuth(apiKey, apiSecret, passphrase string) {
	ws.apiKey = apiKey
	ws.apiSecret = apiSecret
	ws.passphrase = passphrase
}

func (ws *WSUserClient) Listen(ctx context.Context) error {
	if ws.WSClient == nil {
		return fmt.Errorf("websocket not connected")
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, message, err := ws.ReadMessage()
			if err != nil {
				return err
			}

			var arr []json.RawMessage
			if err := json.Unmarshal(message, &arr); err == nil {
				for _, elem := range arr {
					ws.dispatchOne(elem)
				}
				continue
			}

			ws.dispatchOne(message)
		}
	}
}

func (ws *WSUserClient) dispatchOne(message []byte) {
	var base struct {
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(message, &base); err != nil {
		return
	}

	switch base.EventType {
	case "trade":
		if ws.onTrade != nil {
			var m UserTradeMessage
			if err := json.Unmarshal(message, &m); err == nil {
				ws.onTrade(m)
			}
		}
	case "order":
		if ws.onOrder != nil {
			var m UserOrderMessage
			if err := json.Unmarshal(message, &m); err == nil {
				ws.onOrder(m)
			}
		}
	}
}

func (ws *WSUserClient) Subscribe(channel string) error {
	if ws.WSClient == nil {
		return fmt.Errorf("websocket not connected")
	}

	subMsg := SubscriptionMessage{
		Type: channel,
		Auth: &SubscriptionAuth{
			APIKey:     ws.apiKey,
			Secret:     ws.apiSecret,
			Passphrase: ws.passphrase,
		},
	}

	if err := ws.WriteJSON(subMsg); err != nil {
		return fmt.Errorf("failed to send subscribe message: %w", err)
	}
	return nil
}

func (ws *WSUserClient) Close() error { return ws.WSClient.Close() }
