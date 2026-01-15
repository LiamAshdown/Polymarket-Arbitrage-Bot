package bot

import (
	"context"
	"errors"
	"quadbot/client"
	"quadbot/engine"
	"quadbot/logger"
	"quadbot/market"
	"sync"
	"time"
)

var WSFailedToConnect = errors.New("Failed to connect to websocket")
var WSFailedToListen = errors.New("Failed to listen for incoming messages")

type BotConfig struct {
	BufferTicks float64
	MinEdge     float64

	OrderTimeout time.Duration // Cancel orders after 30 seconds

	MaxPairs int

	// Dynamic quantity sizing
	BaseQuantity    float64 // Base order quantity
	MinQuantity     float64 // Minimum order quantity
	MaxQuantity     float64 // Maximum order quantity
	MaxSpreadBps    float64 // Maximum spread in basis points (100 bps = 1%)
	ScaleWithSpread bool    // Whether to reduce quantity as spread widens
}

type Bot struct {
	engine   engine.ExecutionEngine
	event    *market.Event
	wsClient client.WSClient
	logger   logger.Logger

	config BotConfig

	quotes map[string]Quote

	orderPairManager *OrderPairManager
	positionTracker  *PositionTracker
	portfolio        *Portfolio

	books         map[string]Book
	quotePairChan chan QuotePair

	mutex sync.RWMutex
}

func NewBot(engine engine.ExecutionEngine, event *market.Event, config BotConfig, logger logger.Logger) *Bot {
	bot := &Bot{
		engine: engine,
		event:  event,
		logger: logger,

		config: config,

		books: make(map[string]Book),

		quotes:        make(map[string]Quote),
		quotePairChan: make(chan QuotePair, 500),

		orderPairManager: NewOrderPairManager(),
		positionTracker:  NewPositionTracker(logger),
		portfolio:        NewPortfolio(10.0),
	}

	bot.wsClient = *client.NewWSClient(client.WSClientCallbacks{
		OnPriceChange:    bot.onPriceChange,
		OnLastTradePrice: bot.onLastTradePrice,
		OnBestBidAsk:     bot.onBestBidAsk,
		OnTickSizeChange: bot.onTickSizeChange,
		OnBook:           bot.onBook,
	}, logger)

	return bot
}

func (b *Bot) onPriceChange(msg client.PriceChangeMessage) {
	// Collect all incoming orders first
	incomingOrders := make([]engine.IncomingOrder, 0, len(msg.PriceChanges))
	for _, price := range msg.PriceChanges {
		incomingOrders = append(incomingOrders, engine.IncomingOrder{
			ClobTokenID: price.AssetID,
			Price:       float64(price.Price),
			Size:        float64(price.Size),
			Side:        engine.Side(price.Side),
			Hash:        price.Hash,
		})
	}

	b.processFillsAndHedge(incomingOrders)
}

func (b *Bot) onLastTradePrice(msg client.LastTradePriceMessage) {
}

func (b *Bot) onBestBidAsk(msg client.BestBidAskMessage) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.quotes[msg.AssetID] = Quote{
		ConditionID: msg.Market,
		ClobTokenID: msg.AssetID,
		BestBid:     float64(msg.BestBid),
		BestAsk:     float64(msg.BestAsk),
		Spread:      float64(msg.Spread),
		TimeStamp:   msg.Timestamp.Time(),
	}

	clobToken := b.event.GetClobToken()

	upQuote, upExists := b.quotes[clobToken.UpId]
	downQuote, downExists := b.quotes[clobToken.DownId]

	upBidAsks, upBidAsksExists := b.books[clobToken.UpId]
	downBidAsks, downBidAsksExists := b.books[clobToken.DownId]

	if !upBidAsksExists || !downBidAsksExists {
		return
	}

	if len(upBidAsks.Bids) == 0 || len(upBidAsks.Asks) == 0 ||
		len(downBidAsks.Bids) == 0 || len(downBidAsks.Asks) == 0 {
		return
	}

	upQuote.Asks = upBidAsks.Asks
	upQuote.Bids = upBidAsks.Bids

	downQuote.Asks = downBidAsks.Asks
	downQuote.Bids = downBidAsks.Bids

	if upExists && downExists {
		select {
		case b.quotePairChan <- QuotePair{
			Up:   upQuote,
			Down: downQuote,
		}:
		default:
		}
	}
}

func (b *Bot) onBook(msg client.BookMessage) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	bids := make([]BookLevel, len(msg.Bids))
	for i, bid := range msg.Bids {
		bids[i] = BookLevel{
			Price: float64(bid.Price),
			Size:  float64(bid.Size),
		}
	}

	asks := make([]BookLevel, len(msg.Asks))
	for i, ask := range msg.Asks {
		asks[i] = BookLevel{
			Price: float64(ask.Price),
			Size:  float64(ask.Size),
		}
	}

	b.books[msg.AssetID] = Book{
		Asks: asks,
		Bids: bids,
	}
}

func (b *Bot) onTickSizeChange(msg client.TickSizeChangeMessage) {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.event.SetTick(float64(msg.NewTickSize))
}

func (b *Bot) quote(quotePair QuotePair) {
	if len(b.orderPairManager.GetAll()) > b.config.MaxPairs || len(b.positionTracker.GetAll()) > b.config.MaxPairs {
		return
	}

	buffer := b.config.BufferTicks * b.event.Tick

	upLimit := makerBuyPrice(quotePair.Up.BestBid, quotePair.Up.BestAsk, b.event.Tick)
	downLimit := makerBuyPrice(quotePair.Down.BestBid, quotePair.Down.BestAsk, b.event.Tick)

	upIsTaker := upLimit >= quotePair.Up.BestAsk
	downIsTaker := downLimit >= quotePair.Down.BestAsk

	upMid := (quotePair.Up.BestBid + quotePair.Up.BestAsk) / 2
	downMid := (quotePair.Down.BestBid + quotePair.Down.BestAsk) / 2
	upQuantity := b.calculateOrderQuantity(quotePair.Up.Spread, upMid)
	downQuantity := b.calculateOrderQuantity(quotePair.Down.Spread, downMid)

	quantity := min(upQuantity, downQuantity)

	if quantity <= 0 {
		return
	}

	edgeMaker, _ := EdgeBuyNet(upLimit, downLimit, quantity, buffer, b.event.Fee, upIsTaker, downIsTaker)

	if edgeMaker >= b.config.MinEdge {
		if !b.portfolio.HasAvailable((upLimit + downLimit) * quantity) {
			return
		}

		if !b.checkHedgeLiquidity(quotePair.Down, downLimit, quantity) || !b.checkHedgeLiquidity(quotePair.Up, upLimit, quantity) {
			return
		}

		upPendingOrder := b.engine.PlaceOrder(engine.OrderRequest{
			ClobTokenID: quotePair.Up.ClobTokenID,
			Side:        engine.BUY,
			Price:       upLimit,
			Quantity:    quantity,
			Maker:       true,
		})

		downPendingOrder := b.engine.PlaceOrder(engine.OrderRequest{
			ClobTokenID: quotePair.Down.ClobTokenID,
			Side:        engine.BUY,
			Price:       downLimit,
			Quantity:    quantity,
			Maker:       true,
		})

		b.portfolio.Reserve(upPendingOrder.ID, upPendingOrder.Order.Price*quantity)
		b.portfolio.Reserve(downPendingOrder.ID, downPendingOrder.Order.Price*quantity)

		b.orderPairManager.Add(upPendingOrder, downPendingOrder)
	}
}

func (b *Bot) checkHedgeLiquidity(quote Quote, _ float64, quantity float64) bool {
	if len(quote.Asks) == 0 {
		return false
	}

	availableSize := b.getBookSizeAtPrice(quote.Asks, quote.BestAsk, false)
	minRequired := quantity * 2.0

	return availableSize >= minRequired
}

func (b *Bot) getBookSizeAtPrice(levels []BookLevel, price float64, isBid bool) float64 {
	totalSize := 0.0
	for _, level := range levels {
		if isBid {
			if level.Price >= price {
				totalSize += level.Size
			}
		} else {
			if level.Price <= price {
				totalSize += level.Size
			}
		}
	}
	return totalSize
}

func (b *Bot) calculateOrderQuantity(spread float64, midPrice float64) float64 {
	quantity := b.config.BaseQuantity

	if b.config.ScaleWithSpread && midPrice > 0 {
		spreadBps := (spread / midPrice) * 10000

		if b.config.MaxSpreadBps > 0 && spreadBps > b.config.MaxSpreadBps {
			return 0
		}

		if b.config.MaxSpreadBps > 0 {
			scaleFactor := 1.0 - (spreadBps / b.config.MaxSpreadBps)
			scaleFactor = max(0.0, min(1.0, scaleFactor))

			quantity = b.config.MinQuantity + (b.config.BaseQuantity-b.config.MinQuantity)*scaleFactor
		}
	}

	if b.config.MinQuantity > 0 {
		quantity = max(quantity, b.config.MinQuantity)
	}
	if b.config.MaxQuantity > 0 {
		quantity = min(quantity, b.config.MaxQuantity)
	}

	return quantity
}

func (b *Bot) processFillsAndHedge(incomingOrders []engine.IncomingOrder) {
	pairSnapshot := b.orderPairManager.GetAll()
	for _, pair := range pairSnapshot {
		for _, incomingOrder := range incomingOrders {
			b.checkPairFill(pair, incomingOrder)
		}
	}

	for pairID, pair := range pairSnapshot {
		if b.orderPairManager.IsBothSidesFilled(pair) {
			b.handlePairCompletion(pairID, pair)
		}
	}
}

func (b *Bot) checkPairFill(pair *PendingOrderPair, incomingOrder engine.IncomingOrder) (*engine.FillResult, string) {
	var fillResult *engine.FillResult
	var filledSide string

	if incomingOrder.ClobTokenID == pair.DownOrder.Order.ClobTokenID {
		fillResult = b.engine.CheckFill(pair.DownOrder.ID, incomingOrder)
		if fillResult != nil && fillResult.Filled {
			filledSide = "down"
			pair.DownOrder.Filled = fillResult.FullyFilled
			pair.DownOrder.FilledQuantity = fillResult.TotalFilled
			pair.DownOrder.FillPrice = fillResult.FillPrice
		}
	} else if incomingOrder.ClobTokenID == pair.UpOrder.Order.ClobTokenID {
		fillResult = b.engine.CheckFill(pair.UpOrder.ID, incomingOrder)
		if fillResult != nil && fillResult.Filled {
			filledSide = "up"
			pair.UpOrder.Filled = fillResult.FullyFilled
			pair.UpOrder.FilledQuantity = fillResult.TotalFilled
			pair.UpOrder.FillPrice = fillResult.FillPrice
		}
	}

	return fillResult, filledSide
}

func (b *Bot) handlePairCompletion(pairID string, pair *PendingOrderPair) {
	quantity := pair.UpOrder.FilledQuantity
	totalCost := (pair.UpOrder.FillPrice + pair.DownOrder.FillPrice) * quantity
	expectedPayout := quantity * 1.00
	profit := expectedPayout - totalCost

	// Move reserved funds to spent for both orders
	b.portfolio.Fill(pair.UpOrder.ID, pair.UpOrder.FillPrice*quantity)
	b.portfolio.Fill(pair.DownOrder.ID, pair.DownOrder.FillPrice*quantity)

	b.positionTracker.RecordCompletedPair(quantity, totalCost, profit)
	b.orderPairManager.Remove(pairID)
}

func (b *Bot) hedgePosition(pairID string, filledOrder *engine.PendingOrder, unfilledOrder *engine.PendingOrder, quantity float64) {
	hedgeMarket := unfilledOrder.Order.ClobTokenID

	b.mutex.RLock()
	quote, hasQuote := b.quotes[hedgeMarket]
	b.mutex.RUnlock()

	if !hasQuote || time.Since(quote.TimeStamp) > time.Second {
		b.logger.Error("cannot_hedge", "pair_id", pairID, "market", hedgeMarket)
		return
	}

	takerPrice := quote.BestAsk
	b.engine.PlaceOrder(engine.OrderRequest{
		ClobTokenID: hedgeMarket,
		Side:        engine.BUY,
		Price:       takerPrice,
		Quantity:    quantity,
		Maker:       false,
	})

	b.portfolio.Spend(takerPrice * quantity)

	totalCost := (filledOrder.FillPrice + takerPrice) * quantity
	expectedPayout := quantity * 1.00
	profit := expectedPayout - totalCost

	b.positionTracker.RecordCompletedPair(quantity, totalCost, profit)
}

func (b *Bot) checkStaleOrders(ctx context.Context) {
	ticker := time.NewTicker(b.config.OrderTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.cancelStaleOrders()
		case <-ctx.Done():
			return
		}
	}
}

func (b *Bot) cancelStaleOrders() {
	stalePairs := b.orderPairManager.GetStalePairs(b.config.OrderTimeout)

	for _, pair := range stalePairs {
		upFilled := pair.UpOrder.FilledQuantity
		downFilled := pair.DownOrder.FilledQuantity

		if upFilled > 0 && downFilled > 0 {
			netQty := upFilled - downFilled
			if netQty > 0 {
				b.hedgePosition(pair.ID, &pair.UpOrder, &pair.DownOrder, netQty)
				b.portfolio.Release(pair.DownOrder.ID)
			} else if netQty < 0 {
				b.hedgePosition(pair.ID, &pair.DownOrder, &pair.UpOrder, -netQty)
				b.portfolio.Release(pair.UpOrder.ID)
			}
		} else if upFilled > 0 {
			b.hedgePosition(pair.ID, &pair.UpOrder, &pair.DownOrder, upFilled)
			b.portfolio.Release(pair.DownOrder.ID)
		} else if downFilled > 0 {
			b.hedgePosition(pair.ID, &pair.DownOrder, &pair.UpOrder, downFilled)
			b.portfolio.Release(pair.UpOrder.ID)
		}

		if !pair.UpOrder.Filled {
			b.engine.CancelOrder(pair.UpOrder.ID)
			b.portfolio.Release(pair.UpOrder.ID)
		}

		if !pair.DownOrder.Filled {
			b.engine.CancelOrder(pair.DownOrder.ID)
			b.portfolio.Release(pair.DownOrder.ID)
		}

		b.orderPairManager.Remove(pair.ID)
	}
}

func (b *Bot) Run(ctx context.Context) error {
	clobToken := b.event.GetClobToken()

	if err := b.wsClient.Connect(); err != nil {
		b.logger.Error("Failed to connect to Polymarket CLOB", "error", err)
		return WSFailedToConnect
	}

	b.wsClient.SubscribeToMarket([]string{clobToken.DownId, clobToken.UpId})

	ctx, cancel := context.WithCancel(ctx)

	go func() {
		if err := b.wsClient.Listen(ctx); err != nil {
			b.logger.Error("Failed to listen for websocket", "error", err)
			cancel()
		}
	}()

	go b.checkStaleOrders(ctx)

	for {
		select {
		case pair := <-b.quotePairChan:
			if !b.event.IsActive() {
				continue
			}
			b.quote(pair)

		case <-ctx.Done():
			b.logger.Info("Bot shutting down", "reason", ctx.Err())
			return ctx.Err()
		}
	}
}
