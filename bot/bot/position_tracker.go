package bot

import (
	"quadbot/logger"
	"sync"
	"time"
)

type CompletedPair struct {
	Timestamp      time.Time
	Quantity       float64
	TotalCost      float64
	ExpectedPayout float64
	Profit         float64
}

type PositionTracker struct {
	completedPairs []CompletedPair

	logger logger.Logger

	totalPairsCompleted int
	totalQuantity       float64
	totalCost           float64
	totalProfit         float64

	mu sync.RWMutex
}

func NewPositionTracker(logger logger.Logger) *PositionTracker {
	return &PositionTracker{
		completedPairs: make([]CompletedPair, 0),
		logger:         logger,
	}
}

func (pt *PositionTracker) RecordCompletedPair(quantity, totalCost, profit float64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pair := CompletedPair{
		Timestamp:      time.Now(),
		Quantity:       quantity,
		TotalCost:      totalCost,
		ExpectedPayout: quantity * 1.00,
		Profit:         profit,
	}

	pt.completedPairs = append(pt.completedPairs, pair)
	pt.totalPairsCompleted++
	pt.totalQuantity += quantity
	pt.totalCost += totalCost
	pt.totalProfit += profit

	pt.logger.Info("total_profit", "total", pt.totalProfit)
}

func (p *PositionTracker) GetAll() []CompletedPair {
	return p.completedPairs
}

func (pt *PositionTracker) GetStats() (pairsCompleted int, totalQty, totalCost, totalProfit, avgProfitPerPair float64) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	pairsCompleted = pt.totalPairsCompleted
	totalQty = pt.totalQuantity
	totalCost = pt.totalCost
	totalProfit = pt.totalProfit

	if pairsCompleted > 0 {
		avgProfitPerPair = totalProfit / float64(pairsCompleted)
	}

	return
}

func (pt *PositionTracker) GetRecentPairs(n int) []CompletedPair {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	if n > len(pt.completedPairs) {
		n = len(pt.completedPairs)
	}

	if n == 0 {
		return []CompletedPair{}
	}

	start := len(pt.completedPairs) - n
	recent := make([]CompletedPair, n)
	copy(recent, pt.completedPairs[start:])

	return recent
}
