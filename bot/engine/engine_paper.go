package engine

import (
	"context"
	"quadbot/logger"
	"quadbot/utils"
	"sync"
	"time"
)

type Position struct {
	Qty         float64
	Avg         float64
	RealizedPnL float64
}

type PaperEngine struct {
	*BaseEngine
	mu            sync.RWMutex
	pendingOrders map[string]*PendingOrder
	paperBalance  float64
}

func NewPaperEngine(logger logger.Logger) *PaperEngine {
	return &PaperEngine{
		BaseEngine:    NewBaseEngine(logger),
		pendingOrders: make(map[string]*PendingOrder),
		paperBalance:  100.0,
	}
}

func (e *PaperEngine) Name() string {
	return "paper"
}

func (e *PaperEngine) GetBalance(context.Context) (float64, error) {
	return e.paperBalance, nil
}

func (e *PaperEngine) PlaceOrder(order OrderRequest) PendingOrder {
	e.mu.Lock()
	defer e.mu.Unlock()

	pendingOrder := &PendingOrder{
		ID:             utils.GenerateRandomID(),
		Order:          order,
		PlacedAt:       time.Now(),
		Filled:         false,
		FilledQuantity: 0,
	}

	if !order.Maker {
		pendingOrder.Filled = true
		pendingOrder.FilledQuantity = order.Quantity

		fee := e.GetFee(order.ClobTokenID)
		feeUSDC := utils.CalculateTakerFeeUSDC(order.Price, order.Quantity, fee)
		feePerShare := feeUSDC / order.Quantity

		effectivePrice := order.Price
		if order.Side == BUY {
			effectivePrice = order.Price + feePerShare
		} else {
			effectivePrice = order.Price - feePerShare
		}
		pendingOrder.FillPrice = effectivePrice

		return *pendingOrder
	}

	e.logger.Info("place_order", "price", order.Price, "quantity", order.Quantity)

	e.pendingOrders[pendingOrder.ID] = pendingOrder

	return *pendingOrder
}

func (e *PaperEngine) PlaceBatchOrders(orders []OrderRequest) []PendingOrder {
	pendingOrders := make([]PendingOrder, 0, len(orders))

	for _, order := range orders {
		pendingOrder := e.PlaceOrder(order)
		pendingOrders = append(pendingOrders, pendingOrder)
	}

	return pendingOrders
}

func (e *PaperEngine) CancelOrder(orderID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	_, exists := e.pendingOrders[orderID]

	if !exists {
		return OrderNotExist
	}

	e.logger.Info("cancel_order", "order_id", orderID)

	delete(e.pendingOrders, orderID)

	return nil
}

func (e *PaperEngine) Run(ctx context.Context) error {
	return nil
}

func (e *PaperEngine) CheckFill(orderID string, incomingOrder IncomingOrder) *FillResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	pending, exists := e.pendingOrders[orderID]
	if !exists {
		return nil
	}

	order := pending.Order

	if order.ClobTokenID != incomingOrder.ClobTokenID {
		return nil
	}

	var priceMatches bool

	if order.Side == BUY && incomingOrder.Side == SELL {
		priceMatches = order.Price >= incomingOrder.Price
	} else if order.Side == SELL && incomingOrder.Side == BUY {
		priceMatches = order.Price <= incomingOrder.Price
	} else {
		// Same side = no match
		return nil
	}

	if !priceMatches {
		return nil
	}

	remainingQty := order.Quantity - pending.FilledQuantity
	fillQty := min(remainingQty, incomingOrder.Size)

	pending.FilledQuantity += fillQty
	fullyFilled := pending.FilledQuantity >= order.Quantity

	if fullyFilled {
		pending.Filled = true
		delete(e.pendingOrders, orderID)
	}

	return &FillResult{
		Filled:       true,
		FilledQty:    fillQty,
		TotalFilled:  pending.FilledQuantity,
		RemainingQty: remainingQty,
		FullyFilled:  fullyFilled,
		FillPrice:    order.Price, // Maker fills at their limit price
	}
}
