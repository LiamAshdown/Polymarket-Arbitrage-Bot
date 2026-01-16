package service

import (
	"context"
	"errors"
	"fmt"
	"quadbot/client"
	"quadbot/market"
	"quadbot/utils"
	"strings"
)

var ErrEventNotFound = errors.New("event not found")

type Gamma interface {
	GetEvents(ctx context.Context, tagID string) (client.MarketResponse, error)
}

type EventFinder struct {
	gamma Gamma
}

func NewEventFinder(g Gamma) *EventFinder {
	return &EventFinder{gamma: g}
}

func (s *EventFinder) FindEventBySlug(ctx context.Context, tagID, slug string) (*market.Event, error) {
	resp, err := s.gamma.GetEvents(ctx, tagID)
	if err != nil {
		return nil, err
	}

	for _, m := range resp {
		if strings.EqualFold(m.Slug, slug) {
			return market.NewEvent(m), nil
		}
	}
	return nil, ErrEventNotFound
}

func (s *EventFinder) Find15MBTCEvent(ctx context.Context) (*market.Event, error) {
	return s.FindEventBySlug(ctx, "102467", "btc-updown-15m-"+fmt.Sprintf("%d", utils.CalculateNearest15M().Unix()))
}

func (s *EventFinder) Find15METHEvent(ctx context.Context) (*market.Event, error) {
	return s.FindEventBySlug(ctx, "102467", "eth-updown-15m-"+fmt.Sprintf("%d", utils.CalculateNearest15M().Unix()))
}
