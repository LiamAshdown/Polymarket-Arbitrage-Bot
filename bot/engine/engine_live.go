package engine

import (
	"context"
	"fmt"
	"quadbot/client"
	"quadbot/logger"
	"quadbot/market"
	"quadbot/utils"
	"sync"
	"time"
)

type LiveEngineConfig struct {
	APIAddress    string
	APIKey        string
	APISecret     string
	APIPassphrase string
}

type LiveEngine struct {
	*BaseEngine
	mu            sync.RWMutex
	log           logger.Logger
	cfg           LiveEngineConfig
	trade         *client.PolymarketTradeClient
	userWS        *client.WSUserClient
	signer        client.OrderSigner
	event         market.Event
	nonceMu       sync.Mutex
	cachedNonce   string
	pendingOrders map[string]*PendingOrder
	onFill        func(*PendingOrder)
}

func NewLiveEngine(cfg LiveEngineConfig, signer client.OrderSigner, event market.Event, log logger.Logger) *LiveEngine {
	auth := &client.PolymarketL2Auth{
		Address:    cfg.APIAddress,
		APIKey:     cfg.APIKey,
		Secret:     cfg.APISecret,
		Passphrase: cfg.APIPassphrase,
	}
	tc := client.NewPolymarketTradeClient(auth)

	ws := client.NewWSUserClient(client.WSUserCallbacks{}, cfg.APIKey, cfg.APISecret, cfg.APIPassphrase, log)

	return &LiveEngine{
		BaseEngine:    NewBaseEngine(log),
		log:           log,
		cfg:           cfg,
		trade:         tc,
		userWS:        ws,
		signer:        signer,
		event:         event,
		pendingOrders: make(map[string]*PendingOrder),
	}
}

func (e *LiveEngine) Name() string { return "live" }

func (e *LiveEngine) GetBalance(ctx context.Context) (float64, error) {
	return e.trade.GetBalance(ctx)
}

func (e *LiveEngine) signAndPrepareOrder(order *OrderRequest) (*client.PolymarketOrderRequest, error) {
	if err := e.validateAndRoundOrder(order); err != nil {
		return nil, err
	}

	feeRateBps := e.GetFeeString(order.ClobTokenID)
	if feeRateBps == "0" {
		e.log.Error("fee_not_cached", "token", order.ClobTokenID)
	}

	nonce := "0"
	expiration := int64(0)
	zeroAddress := "0x0000000000000000000000000000000000000000"

	signedOrder, err := e.signer.SignOrder(client.OrderSignParams{
		TokenID:       order.ClobTokenID,
		Side:          string(order.Side),
		Price:         order.Price,
		Size:          order.Quantity,
		Maker:         e.cfg.APIAddress,
		Signer:        e.cfg.APIAddress,
		Taker:         zeroAddress,
		Nonce:         nonce,
		FeeRateBps:    feeRateBps,
		Expiration:    expiration,
		SignatureType: 0,
	})

	if err != nil {
		return nil, err
	}

	orderType := "GTC"
	if !order.Maker {
		orderType = "FOK"
	}

	req := &client.PolymarketOrderRequest{
		Order:     *signedOrder,
		Owner:     e.cfg.APIKey,
		OrderType: orderType,
		PostOnly:  order.Maker,
	}

	return req, nil
}

func (e *LiveEngine) PlaceOrder(order OrderRequest) PendingOrder {
	ctx := context.Background()

	req, err := e.signAndPrepareOrder(&order)
	if err != nil {
		e.log.Error("order_preparation_failed", "token", order.ClobTokenID, "err", err)
		pid := utils.GenerateRandomID()
		po := PendingOrder{ID: pid, Order: order}
		e.mu.Lock()
		e.pendingOrders[pid] = &po
		e.mu.Unlock()
		return po
	}

	resp, err := e.trade.PlaceOrder(ctx, *req)
	if err != nil {
		e.log.Error("place_order_failed", "token", order.ClobTokenID, "err", err)
		pid := utils.GenerateRandomID()
		po := PendingOrder{ID: pid, Order: order}
		e.mu.Lock()
		e.pendingOrders[pid] = &po
		e.mu.Unlock()
		return po
	}

	po := PendingOrder{
		ID:             resp.OrderID,
		Order:          order,
		PlacedAt:       time.Now(),
		Filled:         false,
		FilledQuantity: 0,
		FillPrice:      0,
	}

	// Handle order status
	switch resp.Status {
	case client.OrderStatusMatched:
		// Order filled immediately - mark as filled
		po.Filled = true
		po.FilledQuantity = order.Quantity
		po.FillPrice = order.Price
		e.log.Info("order_matched", "order_id", resp.OrderID, "price", order.Price, "quantity", order.Quantity)

		// Trigger fill callback if set
		if e.onFill != nil {
			e.onFill(&po)
		}
		return po

	case client.OrderStatusLive:
		// Order resting on book - wait for fills via WebSocket
		e.log.Info("order_live", "order_id", resp.OrderID, "status", "resting_on_book")

	case client.OrderStatusDelayed:
		// Order marketable but delayed (anti-manipulation)
		e.log.Warn("order_delayed", "order_id", resp.OrderID, "msg", "order marketable but subject to matching delay")

	case client.OrderStatusUnmatched:
		// Order marketable but failed to delay - still placed
		e.log.Warn("order_unmatched", "order_id", resp.OrderID, "msg", "order marketable but failure delaying, placement successful")

	default:
		e.log.Info("order_placed", "order_id", resp.OrderID, "status", string(resp.Status))
	}

	e.mu.Lock()
	e.pendingOrders[po.ID] = &po
	e.mu.Unlock()

	return po
}

func (e *LiveEngine) PlaceBatchOrders(orders []OrderRequest) []PendingOrder {
	ctx := context.Background()

	if len(orders) == 0 {
		return []PendingOrder{}
	}

	if len(orders) > 15 {
		e.log.Error("batch_limit_exceeded", "count", len(orders), "max", 15)
		return []PendingOrder{}
	}

	requests := make([]client.PolymarketOrderRequest, 0, len(orders))
	pendingOrders := make([]PendingOrder, 0, len(orders))

	for i := range orders {
		req, err := e.signAndPrepareOrder(&orders[i])
		if err != nil {
			e.log.Error("batch_order_preparation_failed", "token", orders[i].ClobTokenID, "err", err)
			continue
		}
		requests = append(requests, *req)
	}

	if len(requests) == 0 {
		e.log.Error("no_valid_orders_to_batch")
		return []PendingOrder{}
	}

	responses, err := e.trade.PlaceBatchOrders(ctx, requests)
	if err != nil {
		e.log.Error("batch_order_failed", "err", err)
		return []PendingOrder{}
	}

	e.mu.Lock()
	for i, resp := range responses {
		if !resp.Success || len(resp.ErrorMsg) > 0 {
			e.log.Error("batch_order_placement_failed",
				"index", i,
				"token", orders[i].ClobTokenID,
				"error", resp.ErrorMsg)
			continue
		}

		po := PendingOrder{
			ID:             resp.OrderID,
			Order:          orders[i],
			PlacedAt:       time.Now(),
			Filled:         false,
			FilledQuantity: 0,
			FillPrice:      0,
		}

		switch resp.Status {
		case client.OrderStatusMatched:
			// Order filled immediately
			po.Filled = true
			po.FilledQuantity = orders[i].Quantity
			po.FillPrice = orders[i].Price
			e.log.Info("batch_order_matched",
				"order_id", resp.OrderID,
				"index", i,
				"price", orders[i].Price,
				"quantity", orders[i].Quantity)

			// Trigger fill callback if order matched immediately
			if e.onFill != nil {
				pointerCopy := po
				e.onFill(&pointerCopy)
			}

		case client.OrderStatusLive:
			e.log.Info("batch_order_live",
				"order_id", resp.OrderID,
				"index", i,
				"status", "resting_on_book")

		case client.OrderStatusDelayed:
			e.log.Warn("batch_order_delayed",
				"order_id", resp.OrderID,
				"index", i,
				"msg", "order marketable but delayed")

		case client.OrderStatusUnmatched:
			e.log.Warn("batch_order_unmatched",
				"order_id", resp.OrderID,
				"index", i,
				"msg", "order placement successful but delayed")

		default:
			e.log.Info("batch_order_placed",
				"order_id", resp.OrderID,
				"status", string(resp.Status),
				"index", i)
		}

		e.pendingOrders[po.ID] = &po
		pendingOrders = append(pendingOrders, po)
	}
	e.mu.Unlock()

	return pendingOrders
}

func (e *LiveEngine) CancelOrder(orderID string) error {
	ctx := context.Background()
	_, err := e.trade.CancelOrder(ctx, orderID)
	if err != nil {
		e.log.Error("cancel_order_failed", "order_id", orderID, "err", err)
		return err
	}

	e.mu.Lock()
	if po, ok := e.pendingOrders[orderID]; ok {
		po.Cancelling = true
		e.log.Info("order_cancellation_requested", "order_id", orderID)
	}
	e.mu.Unlock()
	return nil
}

func (e *LiveEngine) CheckFill(orderID string, _ IncomingOrder) *FillResult {
	return nil
}

func (e *LiveEngine) Run(ctx context.Context) error {
	clobToken := e.event.GetClobToken()
	tokenIDs := []string{clobToken.UpId, clobToken.DownId}
	if err := e.PreloadMarketData(ctx, tokenIDs); err != nil {
		e.log.Error("market_data_preload_failed", "err", err)
		return err
	}

	if err := e.userWS.Connect(); err != nil {
		e.log.Error("user_ws_connect_failed", "err", err)
		return err
	}

	e.userWS.OnTrade(func(t client.UserTradeMessage) {
		e.log.Info("trade_event",
			"trade_id", t.ID,
			"taker_order_id", t.TakerOrderID,
			"side", t.Side,
			"size", t.Size,
			"price", t.Price,
			"status", t.Status)

		if t.Status != "MATCHED" && t.Status != "MINED" && t.Status != "CONFIRMED" {
			return
		}

		e.mu.Lock()
		defer e.mu.Unlock()

		if po, ok := e.pendingOrders[t.TakerOrderID]; ok {
			oldFilled := po.FilledQuantity
			po.FilledQuantity = float64(t.Size)
			po.FillPrice = float64(t.Price)
			po.Filled = po.FilledQuantity >= po.Order.Quantity

			if po.Filled {
				delete(e.pendingOrders, t.TakerOrderID)
			}

			if po.FilledQuantity > oldFilled && e.onFill != nil {
				e.onFill(po)
			}
		}

		for _, makerOrder := range t.MakerOrders {
			if po, ok := e.pendingOrders[makerOrder.OrderID]; ok {
				oldFilled := po.FilledQuantity
				po.FilledQuantity += float64(makerOrder.MatchedAmount)
				po.FillPrice = float64(makerOrder.Price)
				po.Filled = po.FilledQuantity >= po.Order.Quantity

				if po.Filled {
					delete(e.pendingOrders, makerOrder.OrderID)
				}

				if po.FilledQuantity > oldFilled && e.onFill != nil {
					e.onFill(po)
				}
			}
		}
	})

	e.userWS.OnOrder(func(o client.UserOrderMessage) {
		e.log.Info("order_event",
			"order_id", o.ID,
			"type", o.Type,
			"side", o.Side,
			"price", o.Price,
			"size_matched", o.SizeMatched)

		if o.Type == "CANCELLATION" {
			e.mu.Lock()
			if _, ok := e.pendingOrders[o.ID]; ok {
				delete(e.pendingOrders, o.ID)
				e.log.Info("order_cancelled", "order_id", o.ID)
			}
			e.mu.Unlock()
		}
	})

	go func() {
		if err := e.userWS.Listen(ctx); err != nil {
			e.log.Error("user_ws_listen_failed", "err", err)
			panic(err)
		}
	}()

	e.userWS.Subscribe("user")

	return nil
}

func (e *LiveEngine) validateAndRoundOrder(order *OrderRequest) error {
	tickSize, minSize, ok := e.GetTickAndMinSize(order.ClobTokenID)

	if !ok {
		ctx := context.Background()
		fetchedTick, fetchedMin, err := e.clobClient.GetTickSizeAndMin(ctx, order.ClobTokenID)
		if err != nil {
			e.log.Error("failed_to_fetch_market_rules", "token", order.ClobTokenID, "err", err)
			return err
		}

		e.CacheTickAndMinSize(order.ClobTokenID, fetchedTick, fetchedMin)
		tickSize = fetchedTick
		minSize = fetchedMin
	}

	if order.Quantity < minSize {
		return fmt.Errorf("order size below minimum: %f < %f", order.Quantity, minSize)
	}

	if tickSize > 0 {
		order.Price = utils.RoundToTick(order.Price, tickSize)
	}

	return nil
}

func (e *LiveEngine) SetOnFill(callback func(*PendingOrder)) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onFill = callback
}
