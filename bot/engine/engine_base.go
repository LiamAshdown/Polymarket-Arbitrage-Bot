package engine

import (
	"context"
	"quadbot/client"
	"quadbot/logger"
	"strconv"
	"sync"
)

type BaseEngine struct {
	mu            sync.RWMutex
	clobClient    *client.ClobClient
	feeBpsCache   map[string]int
	tickSizeCache map[string]float64
	minSizeCache  map[string]float64
	logger        logger.Logger
}

func NewBaseEngine(logger logger.Logger) *BaseEngine {
	return &BaseEngine{
		clobClient:    client.NewClobClient(),
		feeBpsCache:   make(map[string]int),
		tickSizeCache: make(map[string]float64),
		minSizeCache:  make(map[string]float64),
		logger:        logger,
	}
}

func (b *BaseEngine) GetFee(tokenID string) int {
	b.mu.RLock()
	if fee, ok := b.feeBpsCache[tokenID]; ok {
		b.mu.RUnlock()
		return fee
	}
	b.mu.RUnlock()

	ctx := context.Background()
	fee, err := b.clobClient.GetFeeRateBps(ctx, tokenID)
	if err != nil {
		b.logger.Error("fee_fetch_failed", "token", tokenID, "err", err)
		return 0
	}

	b.mu.Lock()
	b.feeBpsCache[tokenID] = fee
	b.mu.Unlock()

	return fee
}

func (b *BaseEngine) GetTick(tokenID string) float64 {
	b.mu.RLock()
	if tick, ok := b.tickSizeCache[tokenID]; ok {
		b.mu.RUnlock()
		return tick
	}
	b.mu.RUnlock()

	ctx := context.Background()
	tick, _, err := b.clobClient.GetTickSizeAndMin(ctx, tokenID)
	if err != nil {
		b.logger.Error("tick_fetch_failed", "token", tokenID, "err", err)
		return 0
	}

	b.mu.Lock()
	b.tickSizeCache[tokenID] = tick
	b.mu.Unlock()

	return tick
}

func (b *BaseEngine) UpdateMarketData(tokenID string, tickSize float64, feeBps int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.feeBpsCache[tokenID] = feeBps
	b.tickSizeCache[tokenID] = tickSize
	b.logger.Info("market_data_updated", "token", tokenID, "tick_size", tickSize, "fee_bps", feeBps)
}

func (b *BaseEngine) PreloadMarketData(ctx context.Context, tokenIDs []string) error {
	for _, tokenID := range tokenIDs {
		feeBps, err := b.clobClient.GetFeeRateBps(ctx, tokenID)
		if err != nil {
			b.logger.Error("failed_to_preload_fee", "token", tokenID, "err", err)
			return err
		}

		tickSize, minSize, err := b.clobClient.GetTickSizeAndMin(ctx, tokenID)
		if err != nil {
			b.logger.Error("failed_to_preload_market_rules", "token", tokenID, "err", err)
			return err
		}

		b.mu.Lock()
		b.feeBpsCache[tokenID] = feeBps
		b.tickSizeCache[tokenID] = tickSize
		b.minSizeCache[tokenID] = minSize
		b.mu.Unlock()

		b.logger.Info("market_data_preloaded", "token", tokenID, "fee_bps", feeBps, "tick_size", tickSize, "min_size", minSize)
	}

	return nil
}

func (b *BaseEngine) GetFeeString(tokenID string) string {
	return strconv.Itoa(b.GetFee(tokenID))
}

func (b *BaseEngine) GetTickAndMinSize(tokenID string) (tickSize float64, minSize float64, ok bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	tick, tickOk := b.tickSizeCache[tokenID]
	min, minOk := b.minSizeCache[tokenID]

	if tickOk && minOk {
		return tick, min, true
	}
	return 0, 0, false
}

func (b *BaseEngine) CacheTickAndMinSize(tokenID string, tickSize float64, minSize float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tickSizeCache[tokenID] = tickSize
	b.minSizeCache[tokenID] = minSize
}
