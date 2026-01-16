package bot

import (
	"fmt"
	"quadbot/engine"
	"sync"
	"time"
)

type OrderPairManager struct {
	pendingPairs map[string]*PendingOrderPair
	mu           sync.RWMutex
	pairCounter  int
}

func NewOrderPairManager() *OrderPairManager {
	return &OrderPairManager{
		pendingPairs: make(map[string]*PendingOrderPair),
	}
}

func (opm *OrderPairManager) Add(upOrder, downOrder engine.PendingOrder) string {
	opm.mu.Lock()
	defer opm.mu.Unlock()

	opm.pairCounter++
	pairID := generatePairID(opm.pairCounter)

	opm.pendingPairs[pairID] = &PendingOrderPair{
		ID:        pairID,
		UpOrder:   upOrder,
		DownOrder: downOrder,
	}

	return pairID
}

func (opm *OrderPairManager) Get(pairID string) (*PendingOrderPair, bool) {
	opm.mu.RLock()
	defer opm.mu.RUnlock()
	pair, exists := opm.pendingPairs[pairID]
	return pair, exists
}

func (opm *OrderPairManager) Remove(pairID string) {
	opm.mu.Lock()
	defer opm.mu.Unlock()
	delete(opm.pendingPairs, pairID)
}

func (opm *OrderPairManager) Count() int {
	opm.mu.RLock()
	defer opm.mu.RUnlock()
	return len(opm.pendingPairs)
}

func (opm *OrderPairManager) GetAll() map[string]*PendingOrderPair {
	opm.mu.RLock()
	defer opm.mu.RUnlock()

	copy := make(map[string]*PendingOrderPair, len(opm.pendingPairs))
	for k, v := range opm.pendingPairs {
		copy[k] = v
	}
	return copy
}

func (opm *OrderPairManager) GetStalePairs(timeout time.Duration) []*PendingOrderPair {
	opm.mu.RLock()
	defer opm.mu.RUnlock()

	now := time.Now()
	var stale []*PendingOrderPair

	for _, pair := range opm.pendingPairs {
		upAge := now.Sub(pair.UpOrder.PlacedAt)
		downAge := now.Sub(pair.DownOrder.PlacedAt)

		if upAge > timeout || downAge > timeout {
			stale = append(stale, pair)
		}
	}

	return stale
}

func (opm *OrderPairManager) IsBothSidesFilled(pair *PendingOrderPair) bool {
	return pair.UpOrder.Filled && pair.DownOrder.Filled
}

func (opm *OrderPairManager) UpdateOrderFill(orderID string, filled bool, filledQty float64, fillPrice float64) (pairID string, bothFilled bool) {
	opm.mu.Lock()
	defer opm.mu.Unlock()

	for id, pair := range opm.pendingPairs {
		if pair.UpOrder.ID == orderID {
			pair.UpOrder.Filled = filled
			pair.UpOrder.FilledQuantity = filledQty
			pair.UpOrder.FillPrice = fillPrice
			return id, pair.UpOrder.Filled && pair.DownOrder.Filled
		} else if pair.DownOrder.ID == orderID {
			pair.DownOrder.Filled = filled
			pair.DownOrder.FilledQuantity = filledQty
			pair.DownOrder.FillPrice = fillPrice
			return id, pair.UpOrder.Filled && pair.DownOrder.Filled
		}
	}
	return "", false
}

func generatePairID(counter int) string {
	return fmt.Sprintf("PAIR-%d", counter)
}
