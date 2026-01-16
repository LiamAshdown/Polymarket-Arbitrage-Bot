package client

import (
	"context"
	"encoding/json"
	"fmt"
	"quadbot/logger"
)

type WSMarketClient struct {
	*WSClient
	onPriceChange    func(PriceChangeMessage)
	onTickSizeChange func(TickSizeChangeMessage)
	onLastTradePrice func(LastTradePriceMessage)
	onBestBidAsk     func(BestBidAskMessage)
	onBook           func(BookMessage)
}

type WSMarketCallbacks struct {
	OnPriceChange    func(PriceChangeMessage)
	OnTickSizeChange func(TickSizeChangeMessage)
	OnLastTradePrice func(LastTradePriceMessage)
	OnBestBidAsk     func(BestBidAskMessage)
	OnBook           func(BookMessage)
}

func NewWSMarketClient(callbacks WSMarketCallbacks, log logger.Logger) *WSMarketClient {
	base := NewWSClient("wss://ws-subscriptions-clob.polymarket.com/ws/market", log)
	return &WSMarketClient{
		WSClient:         base,
		onPriceChange:    callbacks.OnPriceChange,
		onTickSizeChange: callbacks.OnTickSizeChange,
		onLastTradePrice: callbacks.OnLastTradePrice,
		onBestBidAsk:     callbacks.OnBestBidAsk,
		onBook:           callbacks.OnBook,
	}
}

func (ws *WSMarketClient) SubscribeToMarket(clobTokenIDs []string) error {
	if ws.WSClient == nil {
		return fmt.Errorf("websocket not connected")
	}

	subMsg := WSMarketSubscribeMessage{
		Type:                  "subscribe",
		Assets:                clobTokenIDs,
		CustomFeaturesEnabled: true,
	}
	if err := ws.WriteJSON(subMsg); err != nil {
		return fmt.Errorf("failed to send subscribe message: %w", err)
	}
	ws.logger.Info("Subscribed to token IDs", "count", len(clobTokenIDs))
	return nil
}

func (ws *WSMarketClient) dispatchOne(message []byte) {
	var msgType WSMessage
	if err := json.Unmarshal(message, &msgType); err != nil {
		return
	}

	switch msgType.EventType {
	case "price_change":
		if ws.onPriceChange != nil {
			var m PriceChangeMessage
			if err := json.Unmarshal(message, &m); err == nil {
				ws.onPriceChange(m)
			}
		}
	case "tick_size_change":
		if ws.onTickSizeChange != nil {
			var m TickSizeChangeMessage
			if err := json.Unmarshal(message, &m); err == nil {
				ws.onTickSizeChange(m)
			}
		}
	case "last_trade_price":
		if ws.onLastTradePrice != nil {
			var m LastTradePriceMessage
			if err := json.Unmarshal(message, &m); err == nil {
				ws.onLastTradePrice(m)
			}
		}
	case "best_bid_ask":
		if ws.onBestBidAsk != nil {
			var m BestBidAskMessage
			if err := json.Unmarshal(message, &m); err == nil {
				ws.onBestBidAsk(m)
			}
		}
	case "book":
		if ws.onBook != nil {
			var m BookMessage
			if err := json.Unmarshal(message, &m); err == nil {
				ws.onBook(m)
			}
		}
	}
}

func (ws *WSMarketClient) Listen(ctx context.Context) error {
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

			var raw json.RawMessage
			if err := json.Unmarshal(message, &raw); err != nil {
				continue
			}

			if len(raw) > 0 && raw[0] == '[' {
				var arr []json.RawMessage
				if err := json.Unmarshal(message, &arr); err != nil {
					continue
				}
				for _, elem := range arr {
					ws.dispatchOne(elem)
				}
				continue
			}

			ws.dispatchOne(message)
		}
	}

}

func (ws *WSMarketClient) Close() error { return ws.WSClient.Close() }
