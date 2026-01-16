package engine

import (
	"errors"
	"time"
)

type Side string

var OrderNotExist = errors.New("order does not exist")

const (
	BUY  Side = "BUY"
	SELL Side = "SELL"
)

type OrderRequest struct {
	ClobTokenID string
	Side        Side
	Price       float64
	Quantity    float64
	Maker       bool
}

type PendingOrder struct {
	ID             string
	Order          OrderRequest
	PlacedAt       time.Time
	Filled         bool
	FilledQuantity float64
	FillPrice      float64 // Actual fill price including fees for taker orders
	Cancelling     bool    // Order cancellation requested, waiting for confirmation
}

type IncomingOrder struct {
	ClobTokenID string
	Price       float64
	Size        float64
	Side        Side
	Hash        string
}

type FillResult struct {
	Filled       bool    // Did any fill occur?
	FilledQty    float64 // How much was filled in this specific event
	TotalFilled  float64 // Total filled so far
	RemainingQty float64 // How much is left
	FullyFilled  bool    // Is the order completely filled?
	FillPrice    float64 // Price at which this fill occurred
}
