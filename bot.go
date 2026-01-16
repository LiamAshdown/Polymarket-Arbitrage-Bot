package bot

import (
	"context"
	"errors"
	"fmt"
	"quadbot/client"
	"quadbot/engine"
	"quadbot/logger"
	"quadbot/market"
	"sync"
	"time"
)

const (
	quoteChannelBuffer       = 500
	quoteStaleTimeout        = time.Second
	hedgeLiquidityMultiplier = 2.0
	binaryMarketPayout       = 1.00
)

var (
	ErrWSFailedToConnect = errors.New("failed to connect to websocket")
	ErrWSFailedToListen  = errors.New("failed to listen for incoming messages")
)

type BotConfig struct {
	BufferTicks  float64
	MinEdge      float64
	OrderTimeout time.Duration
	MaxPairs     int

	BaseQuantity    float64
	MinQuantity     float64
	MaxQuantity     float64
	MaxSpreadBps    float64
	ScaleWithSpread bool
}

type Bot struct {
	engine   engine.ExecutionEngine
	event    *market.Event
	wsClient client.WSMarketClient
	logger   logger.Logger
	config   BotConfig

	quotes        map[string]Quote
	books         map[string]Book
	quotePairChan chan QuotePair

	orderPairManager *OrderPairManager
	positionTracker  *PositionTracker
	portfolio        *Portfolio
	hedgeOrders      map[string]*HedgeOrderInfo

	mutex sync.RWMutex
}

type HedgeOrderInfo struct {
	OrderID       string
	PairID        string
	ExpectedPrice float64
	Quantity      float64
	FilledOrder   engine.PendingOrder
}

func NewBot(engine engine.ExecutionEngine, event *market.Event, config BotConfig, logger logger.Logger) *Bot {
	bot := &Bot{
		engine: engine,
		event:  event,
		logger: logger,
		config: config,

		quotes:        make(map[string]Quote),
		books:         make(map[string]Book),
		quotePairChan: make(chan QuotePair, quoteChannelBuffer),

		orderPairManager: NewOrderPairManager(),
		positionTracker:  NewPositionTracker(logger),
		hedgeOrders:      make(map[string]*HedgeOrderInfo),
	}

	bot.wsClient = *client.NewWSMarketClient(client.WSMarketCallbacks{
		OnPriceChange:    bot.onPriceChange,
		OnBestBidAsk:     bot.onBestBidAsk,
		OnTickSizeChange: bot.onTickSizeChange,
		OnBook:           bot.onBook,
	}, logger)

	return bot
}

func (b *Bot) onPriceChange(msg client.PriceChangeMessage) {
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
	b.checkPaperFills(incomingOrders)
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

	if quotePair, ok := b.tryBuildQuotePair(); ok {
		select {
		case b.quotePairChan <- quotePair:
		default:
		}
	}
}

func (b *Bot) tryBuildQuotePair() (QuotePair, bool) {
	clobToken := b.event.GetClobToken()

	upQuote, upExists := b.quotes[clobToken.UpId]
	downQuote, downExists := b.quotes[clobToken.DownId]
	if !upExists || !downExists {
		return QuotePair{}, false
	}

	upBook, upBookExists := b.books[clobToken.UpId]
	downBook, downBookExists := b.books[clobToken.DownId]
	if !upBookExists || !downBookExists {
		return QuotePair{}, false
	}

	if !b.hasValidBookDepth(upBook) || !b.hasValidBookDepth(downBook) {
		return QuotePair{}, false
	}

	return QuotePair{Up: upQuote, Down: downQuote}, true
}

func (b *Bot) hasValidBookDepth(book Book) bool {
	return len(book.Bids) > 0 && len(book.Asks) > 0
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

func (b *Bot) onTickSizeChange(msg client.TickSizeChangeMessage) {}

func (b *Bot) quote(quotePair QuotePair) {
	if b.hasReachedPositionLimits() {
		return
	}

	pricing := b.calculatePricing(quotePair)
	quantity := b.calculateOptimalQuantity(quotePair)
	if quantity <= 0 {
		return
	}

	edge := b.calculateEdge(quotePair, pricing, quantity)
	if edge < b.config.MinEdge {
		return
	}

	if !b.canPlaceOrders(quotePair, pricing, quantity) {
		return
	}

	b.placeOrderPair(quotePair, pricing, quantity)
}

func (b *Bot) hasReachedPositionLimits() bool {
	return len(b.orderPairManager.GetAll()) >= b.config.MaxPairs ||
		len(b.positionTracker.GetAll()) >= b.config.MaxPairs
}

type pricingParams struct {
	upLimit     float64
	downLimit   float64
	upTick      float64
	downTick    float64
	buffer      float64
	upIsTaker   bool
	downIsTaker bool
}

func (b *Bot) calculatePricing(quotePair QuotePair) pricingParams {
	upTick := b.engine.GetTick(quotePair.Up.ClobTokenID)
	downTick := b.engine.GetTick(quotePair.Down.ClobTokenID)

	tick := max(upTick, downTick)
	buffer := b.config.BufferTicks * tick

	upLimit := makerBuyPrice(quotePair.Up.BestBid, quotePair.Up.BestAsk, upTick)
	downLimit := makerBuyPrice(quotePair.Down.BestBid, quotePair.Down.BestAsk, downTick)

	return pricingParams{
		upLimit:     upLimit,
		downLimit:   downLimit,
		upTick:      upTick,
		downTick:    downTick,
		buffer:      buffer,
		upIsTaker:   upLimit >= quotePair.Up.BestAsk,
		downIsTaker: downLimit >= quotePair.Down.BestAsk,
	}
}

func (b *Bot) calculateOptimalQuantity(quotePair QuotePair) float64 {
	upMid := (quotePair.Up.BestBid + quotePair.Up.BestAsk) / 2
	downMid := (quotePair.Down.BestBid + quotePair.Down.BestAsk) / 2

	upQuantity := b.calculateOrderQuantity(quotePair.Up.Spread, upMid)
	downQuantity := b.calculateOrderQuantity(quotePair.Down.Spread, downMid)

	return min(upQuantity, downQuantity)
}

// calculateEdge computes the expected profit edge for the order pair
func (b *Bot) calculateEdge(quotePair QuotePair, pricing pricingParams, quantity float64) float64 {
	edge, _ := EdgeBuyNet(
		pricing.upLimit,
		pricing.downLimit,
		quantity,
		pricing.buffer,
		b.engine.GetFee(quotePair.Up.ClobTokenID),
		pricing.upIsTaker,
		pricing.downIsTaker,
	)
	return edge
}

func (b *Bot) canPlaceOrders(quotePair QuotePair, pricing pricingParams, quantity float64) bool {
	totalCost := (pricing.upLimit + pricing.downLimit) * quantity

	if !b.portfolio.HasAvailable(totalCost) {
		return false
	}

	b.mutex.RLock()
	clobToken := b.event.GetClobToken()
	upBook, upExists := b.books[clobToken.UpId]
	downBook, downExists := b.books[clobToken.DownId]
	b.mutex.RUnlock()

	if !upExists || !downExists {
		return false
	}

	upQuote := Quote{Asks: upBook.Asks, BestAsk: quotePair.Up.BestAsk}
	downQuote := Quote{Asks: downBook.Asks, BestAsk: quotePair.Down.BestAsk}

	// we need to make sure if our maker order is not filled, we can immediantly hedge instead
	return b.hasHedgeLiquidity(upQuote, quantity) && b.hasHedgeLiquidity(downQuote, quantity)
}

func (b *Bot) placeOrderPair(quotePair QuotePair, pricing pricingParams, quantity float64) {
	upOrder := b.engine.PlaceOrder(engine.OrderRequest{
		ClobTokenID: quotePair.Up.ClobTokenID,
		Side:        engine.BUY,
		Price:       pricing.upLimit,
		Quantity:    quantity,
		Maker:       true,
	})

	downOrder := b.engine.PlaceOrder(engine.OrderRequest{
		ClobTokenID: quotePair.Down.ClobTokenID,
		Side:        engine.BUY,
		Price:       pricing.downLimit,
		Quantity:    quantity,
		Maker:       true,
	})

	b.portfolio.Reserve(upOrder.ID, upOrder.Order.Price*upOrder.Order.Quantity)
	b.portfolio.Reserve(downOrder.ID, downOrder.Order.Price*downOrder.Order.Quantity)

	b.orderPairManager.Add(upOrder, downOrder)
}

func (b *Bot) hasHedgeLiquidity(quote Quote, quantity float64) bool {
	if len(quote.Asks) == 0 {
		return false
	}

	availableSize := b.getBookSizeAtPrice(quote.Asks, quote.BestAsk, false)
	minRequired := quantity * hedgeLiquidityMultiplier

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

// ============================================================================
// Paper Trading Fill Simulation
// ============================================================================

// checkPaperFills simulates order fills for paper trading mode
func (b *Bot) checkPaperFills(incomingOrders []engine.IncomingOrder) {
	if b.engine.Name() != "paper" {
		return
	}

	for _, pair := range b.orderPairManager.GetAll() {
		for _, incomingOrder := range incomingOrders {
			b.checkAndUpdatePairFill(pair, incomingOrder)
		}

		if pair.UpOrder.Filled && pair.DownOrder.Filled {
			b.handlePairCompletion(pair.ID, pair)
			b.orderPairManager.Remove(pair.ID)
		}
	}
}

func (b *Bot) checkAndUpdatePairFill(pair *PendingOrderPair, incomingOrder engine.IncomingOrder) {
	if incomingOrder.ClobTokenID == pair.DownOrder.Order.ClobTokenID {
		if fillResult := b.engine.CheckFill(pair.DownOrder.ID, incomingOrder); fillResult != nil && fillResult.Filled {
			pair.DownOrder.Filled = fillResult.FullyFilled
			pair.DownOrder.FilledQuantity = fillResult.TotalFilled
			pair.DownOrder.FillPrice = fillResult.FillPrice
		}
	} else if incomingOrder.ClobTokenID == pair.UpOrder.Order.ClobTokenID {
		if fillResult := b.engine.CheckFill(pair.UpOrder.ID, incomingOrder); fillResult != nil && fillResult.Filled {
			pair.UpOrder.Filled = fillResult.FullyFilled
			pair.UpOrder.FilledQuantity = fillResult.TotalFilled
			pair.UpOrder.FillPrice = fillResult.FillPrice
		}
	}
}

func (b *Bot) handlePairCompletion(pairID string, pair *PendingOrderPair) {
	quantity := min(pair.UpOrder.FilledQuantity, pair.DownOrder.FilledQuantity)
	if quantity <= 0 {
		return
	}

	position := calculateBinaryPnL(pair.UpOrder.FillPrice, pair.DownOrder.FillPrice, quantity)

	b.portfolio.Fill(pair.UpOrder.ID, pair.UpOrder.FillPrice*quantity)
	b.portfolio.Fill(pair.DownOrder.ID, pair.DownOrder.FillPrice*quantity)

	b.positionTracker.RecordCompletedPair(position.Quantity, position.Cost, position.Profit)
}

func (b *Bot) hedgePosition(pairID string, filledOrder *engine.PendingOrder, unfilledOrder *engine.PendingOrder, quantity float64) {
	if quantity <= 0 {
		b.logger.Warn("hedge_zero_quantity", "pair_id", pairID)
		return
	}

	hedgeMarket := unfilledOrder.Order.ClobTokenID

	b.mutex.RLock()
	quote, hasQuote := b.quotes[hedgeMarket]
	b.mutex.RUnlock()

	if !hasQuote || time.Since(quote.TimeStamp) > quoteStaleTimeout {
		b.logger.Error("cannot_hedge_stale_quote", "pair_id", pairID, "market", hedgeMarket)
		return
	}

	takerPrice := quote.BestAsk
	hedgeOrder := b.engine.PlaceOrder(engine.OrderRequest{
		ClobTokenID: hedgeMarket,
		Side:        engine.BUY,
		Price:       takerPrice,
		Quantity:    quantity,
		Maker:       false,
	})

	if b.engine.Name() == "paper" {
		cost := takerPrice * quantity
		if !b.portfolio.HasAvailable(cost) {
			b.logger.Error("insufficient_funds_for_hedge", "required", cost, "pair_id", pairID)
			return
		}
		b.portfolio.Spend(cost)
		position := calculateBinaryPnL(filledOrder.FillPrice, takerPrice, quantity)
		b.positionTracker.RecordCompletedPair(position.Quantity, position.Cost, position.Profit)
	} else {
		b.portfolio.Reserve(hedgeOrder.ID, takerPrice*quantity)
		b.mutex.Lock()
		b.hedgeOrders[hedgeOrder.ID] = &HedgeOrderInfo{
			OrderID:       hedgeOrder.ID,
			PairID:        pairID,
			ExpectedPrice: takerPrice,
			Quantity:      quantity,
			FilledOrder:   *filledOrder,
		}
		b.mutex.Unlock()
	}
}

func (b *Bot) handleLiveFill(filledOrder *engine.PendingOrder) {
	b.mutex.RLock()
	hedgeInfo, isHedge := b.hedgeOrders[filledOrder.ID]
	b.mutex.RUnlock()

	if isHedge {
		b.portfolio.Fill(hedgeInfo.OrderID, filledOrder.FillPrice*filledOrder.FilledQuantity)

		position := calculateBinaryPnL(hedgeInfo.FilledOrder.FillPrice, filledOrder.FillPrice, filledOrder.FilledQuantity)
		b.positionTracker.RecordCompletedPair(position.Quantity, position.Cost, position.Profit)

		b.mutex.Lock()
		delete(b.hedgeOrders, filledOrder.ID)
		b.mutex.Unlock()
		return
	}

	pairID, bothFilled := b.orderPairManager.UpdateOrderFill(
		filledOrder.ID,
		filledOrder.Filled,
		filledOrder.FilledQuantity,
		filledOrder.FillPrice,
	)

	if pairID == "" {
		return
	}

	if bothFilled {
		pair, exists := b.orderPairManager.Get(pairID)
		if exists {
			b.handlePairCompletion(pairID, pair)
			b.orderPairManager.Remove(pairID)
		}
	}
}

func (b *Bot) setupLiveEngineIfNeeded() {
	if liveEngine, ok := b.engine.(interface {
		SetOnFill(func(*engine.PendingOrder))
	}); ok {
		liveEngine.SetOnFill(b.handleLiveFill)
	}
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

	if len(stalePairs) > 0 {
		b.logger.Warn("stale_orders", "count", fmt.Sprintf("found %d stale pairs, hedging positions", len(stalePairs)))
	}

	for _, pair := range stalePairs {
		upFilled := pair.UpOrder.FilledQuantity
		downFilled := pair.DownOrder.FilledQuantity

		if upFilled > 0 && downFilled > 0 {
			matchedQty := min(upFilled, downFilled)
			if matchedQty > 0 {
				b.handlePairCompletion(pair.ID, pair)
			}

			unmatchedQty := upFilled - downFilled
			if unmatchedQty > 0 {
				b.hedgePosition(pair.ID, &pair.UpOrder, &pair.DownOrder, unmatchedQty)
			} else if unmatchedQty < 0 {
				b.hedgePosition(pair.ID, &pair.DownOrder, &pair.UpOrder, -unmatchedQty)
			}
		} else if upFilled > 0 {
			b.hedgePosition(pair.ID, &pair.UpOrder, &pair.DownOrder, upFilled)
		} else if downFilled > 0 {
			b.hedgePosition(pair.ID, &pair.DownOrder, &pair.UpOrder, downFilled)
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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	balance, err := b.engine.GetBalance(ctx)
	if err != nil {
		b.logger.Error("failed_to_get_balance", "error", err)
		cancel()
	}

	if balance <= 0 {
		b.logger.Error("not_enough_balance", "msg", "Your balance is 0. Bot cannot run. Shutting down")
		cancel()
	}

	b.portfolio = NewPortfolio(balance)
	b.logger.Info("portfolio_initialized", "balance", balance, "engine", b.engine.Name())

	clobToken := b.event.GetClobToken()

	if err := b.wsClient.Connect(); err != nil {
		b.logger.Error("ws_connect_failed", "error", err)
		cancel()
	}

	b.wsClient.SubscribeToMarket([]string{clobToken.DownId, clobToken.UpId})
	b.setupLiveEngineIfNeeded()

	go func() {
		if err := b.wsClient.Listen(ctx); err != nil {
			b.logger.Error("market_ws_listen", "error", err)
			cancel()
		}
	}()

	if err := b.engine.Run(ctx); err != nil {
		b.logger.Error("engine_run_failed", "error", err)
		cancel()
	}

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
