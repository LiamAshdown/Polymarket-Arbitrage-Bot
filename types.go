package bot

import (
	"quadbot/engine"
	"time"
)

type BookLevel struct {
	Price float64
	Size  float64
}

type Book struct {
	Bids []BookLevel
	Asks []BookLevel
}

type Quote struct {
	ConditionID string
	ClobTokenID string
	BestBid     float64
	BestAsk     float64
	Spread      float64
	TimeStamp   time.Time
	Bids        []BookLevel // Book depth on buy side
	Asks        []BookLevel // Book depth on sell side
}

type QuotePair struct {
	Up   Quote
	Down Quote
}

type PendingOrderPair struct {
	ID        string
	UpOrder   engine.PendingOrder
	DownOrder engine.PendingOrder
}
