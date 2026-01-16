package market

import (
	"quadbot/client"
	"time"
)

type MarketEvent struct {
	ID          string
	Ticker      string
	Slug        string
	Title       string
	Description string
	Active      bool
	Closed      bool
	Market      Market
}

type Event struct {
	MarketEvent MarketEvent
}

type ClobToken struct {
	UpId   string
	DownId string
}

type Market struct {
	ID              string
	Question        string
	Slug            string
	Active          bool
	AcceptingOrders bool
	UpId            string
	DownId          string
	ClobToken       ClobToken
	FeesEnabled     bool
	StartDate       time.Time
	EndDate         time.Time
}

func NewEvent(market client.MarketEvent) *Event {
	markets := market.Markets

	// We're only expecting 1 market
	// If there's more than 1 market, then return nil
	if len(markets) == 0 || len(markets) > 1 {
		return nil
	}

	event := markets[0]
	clobTokenIds := event.ClobTokenIds

	// Since we only care about binary markets
	// If there isn't two tokens (up|down)
	// then we can't work with it
	if len(clobTokenIds) != 2 {
		return nil
	}

	return &Event{
		MarketEvent: MarketEvent{

			ID:          market.ID,
			Ticker:      market.Ticker,
			Slug:        market.Slug,
			Title:       market.Title,
			Description: market.Description,
			Active:      market.Active,
			Closed:      market.Closed,
			Market: Market{
				ID:              event.ID,
				Question:        event.Question,
				Slug:            event.Slug,
				Active:          event.Active,
				AcceptingOrders: event.AcceptingOrders,
				ClobToken: ClobToken{
					UpId:   event.ClobTokenIds[0],
					DownId: event.ClobTokenIds[1],
				},
				FeesEnabled: event.FeesEnabled,
			},
		},
	}
}

func (e *Event) IsActive() bool {
	now := time.Now()

	if e.MarketEvent.Active && !e.MarketEvent.Closed && e.MarketEvent.Market.AcceptingOrders && e.MarketEvent.Market.Active {
		endDate := e.MarketEvent.Market.EndDate.Add(-10 * time.Minute)
		if now.After(e.MarketEvent.Market.StartDate) || now.Equal(e.MarketEvent.Market.StartDate) && now.Before(endDate) {
			return true
		}
	}

	return false
}

func (e *Event) GetClobToken() ClobToken {
	return e.MarketEvent.Market.ClobToken
}
