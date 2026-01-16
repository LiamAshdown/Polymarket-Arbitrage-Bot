package client

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type EscapedArray []string
type StringFloat64 float64
type TimeRFC3339 time.Time
type UnixTimestamp time.Time

// =============================
// CLOB + Gamma Data Types
// =============================

type MarketResponse []MarketEvent

type MarketEvent struct {
	ID          string   `json:"id"`
	Ticker      string   `json:"ticker"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Active      bool     `json:"active"`
	Closed      bool     `json:"closed"`
	Markets     []Market `json:"markets"`
}

type Market struct {
	ID              string       `json:"id"`
	Question        string       `json:"question"`
	Slug            string       `json:"slug"`
	Active          bool         `json:"active"`
	AcceptingOrders bool         `json:"acceptingOrders"`
	ClobTokenIds    EscapedArray `json:"clobTokenIds"`
	FeesEnabled     bool         `json:"feesEnabled"`
	StartDate       TimeRFC3339  `json:"startDate"`
	EndDate         TimeRFC3339  `json:"endDate"`
}

type FeeRateResp struct {
	FeeRateBps int `json:"base_fee"`
}

type OrderSummary struct {
	Price StringFloat64 `json:"price"`
	Size  StringFloat64 `json:"size"`
}

type GetBookResponse struct {
	Market         string         `json:"market"`
	AssetID        string         `json:"asset_id"`
	Timestamp      string         `json:"timestamp"`
	Hash           string         `json:"hash"`
	Bids           []OrderSummary `json:"bids"`
	Asks           []OrderSummary `json:"asks"`
	MinOrderSize   string         `json:"min_order_size"`
	TickSize       StringFloat64  `json:"tick_size"`
	NegRisk        bool           `json:"neg_risk"`
	LastTradePrice string         `json:"last_trade_price"`
}

type GetPriceResponse struct {
	Price string `json:"price"`
}

type ApiKeyResponse struct {
	ApiKey     string `json:"apiKey"`
	Secret     string `json:"secret"`
	Passphrase string `json:"passphrase"`
}

type GeoBlockResponse struct {
	Blocked bool   `json:"blocked"`
	IP      string `json:"ip"`
	Country string `json:"country"`
	Region  string `json:"region"`
}

// =============================
// WebSocket Types
// =============================

type WSMarketSubscribeMessage struct {
	Type                  string   `json:"type"`
	Markets               []string `json:"markets,omitempty"`
	Assets                []string `json:"assets_ids,omitempty"`
	CustomFeaturesEnabled bool     `json:"custom_feature_enabled"`
}

type PriceChange struct {
	AssetID string        `json:"asset_id"`
	Price   StringFloat64 `json:"price"`
	Size    StringFloat64 `json:"size"`
	Side    string        `json:"side"` // "BUY" or "SELL"
	Hash    string        `json:"hash"`
	BestBid StringFloat64 `json:"best_bid"`
	BestAsk StringFloat64 `json:"best_ask"`
}

type PriceChangeMessage struct {
	EventType    string        `json:"event_type"` // "price_change"
	Market       string        `json:"market"`
	PriceChanges []PriceChange `json:"price_changes"`
	Timestamp    UnixTimestamp `json:"timestamp"`
}

type TickSizeChangeMessage struct {
	EventType   string        `json:"event_type"` // "tick_size_change"
	AssetID     string        `json:"asset_id"`
	Market      string        `json:"market"`
	OldTickSize StringFloat64 `json:"old_tick_size"`
	NewTickSize StringFloat64 `json:"new_tick_size"`
	Side        string        `json:"side"`
	Timestamp   UnixTimestamp `json:"timestamp"`
}

type LastTradePriceMessage struct {
	EventType string        `json:"event_type"` // "last_trade_price"
	AssetID   string        `json:"asset_id"`
	Market    string        `json:"market"`
	Price     StringFloat64 `json:"price"`
	Side      string        `json:"side"` // "BUY" or "SELL"
	Size      StringFloat64 `json:"size"`
	Timestamp UnixTimestamp `json:"timestamp"`
}

type BestBidAskMessage struct {
	EventType string        `json:"event_type"` // "best_bid_ask"
	Market    string        `json:"market"`
	AssetID   string        `json:"asset_id"`
	BestBid   StringFloat64 `json:"best_bid"`
	BestAsk   StringFloat64 `json:"best_ask"`
	Spread    StringFloat64 `json:"spread"`
	Timestamp UnixTimestamp `json:"timestamp"`
}

type BookMessage struct {
	EventType string         `json:"event_type"` // "book"
	AssetID   string         `json:"asset_id"`
	Market    string         `json:"market"`
	Bids      []OrderSummary `json:"bids"`
	Asks      []OrderSummary `json:"asks"`
	Timestamp string         `json:"timestamp"`
	Hash      string         `json:"hash"`
}

type WSMessage struct {
	EventType string `json:"event_type"`
}

// =============================
// JSON Unmarshal Methods
// =============================

func (sf *StringFloat64) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*sf = StringFloat64(f)
	return nil
}

func (e *EscapedArray) UnmarshalJSON(data []byte) error {
	s := string(data)
	s = strings.Trim(s, `"`)
	s = strings.ReplaceAll(s, `\\\"`, `"`)
	s = strings.ReplaceAll(s, `\"`, `"`)

	var temp []string
	if err := json.Unmarshal([]byte(s), &temp); err != nil {
		return err
	}
	*e = EscapedArray(temp)
	return nil
}

func (t *TimeRFC3339) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if s == "null" || s == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return err
	}
	*t = TimeRFC3339(parsed)
	return nil
}

func (t TimeRFC3339) Time() time.Time {
	return time.Time(t)
}

func (t TimeRFC3339) Unix() int64 {
	return time.Time(t).Unix()
}

func (ut *UnixTimestamp) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if s == "null" || s == "" {
		return nil
	}
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return err
	}
	*ut = UnixTimestamp(time.Unix(sec, 0))
	return nil
}

func (ut UnixTimestamp) Time() time.Time {
	return time.Time(ut)
}

func (ut UnixTimestamp) Unix() int64 {
	return time.Time(ut).Unix()
}
