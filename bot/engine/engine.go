package engine

import "context"

type ExecutionEngine interface {
	Name() string

	PlaceOrder(order OrderRequest) PendingOrder
	PlaceBatchOrders(orders []OrderRequest) []PendingOrder
	CancelOrder(orderID string) error

	CheckFill(orderID string, incomingOrder IncomingOrder) *FillResult

	GetFee(tokenID string) int
	GetTick(tokenID string) float64
	UpdateMarketData(tokenID string, tickSize float64, feeBps int)
	GetBalance(ctx context.Context) (float64, error)

	Run(ctx context.Context) error
}
