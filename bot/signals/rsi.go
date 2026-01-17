package signals

import "sync"

type RSI struct {
	period     int
	overbought float64
	oversold   float64

	data  map[string]*rsiData
	mutex sync.RWMutex
}

type rsiData struct {
	prices      []float64
	avgGain     float64
	avgLoss     float64
	rsi         float64
	initialized bool
}

func NewRSI(period int, overbought, oversold float64) *RSI {
	return &RSI{
		period:     period,
		overbought: overbought,
		oversold:   oversold,
		data:       make(map[string]*rsiData),
	}
}

func (r *RSI) Name() string {
	return "RSI"
}

func (r *RSI) Update(tokenID string, price float64) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, exists := r.data[tokenID]; !exists {
		r.data[tokenID] = &rsiData{
			prices: make([]float64, 0, r.period+1),
		}
	}

	data := r.data[tokenID]
	data.prices = append(data.prices, price)

	if len(data.prices) < r.period+1 {
		return
	}

	if len(data.prices) > r.period+1 {
		data.prices = data.prices[1:]
	}

	r.calculateRSI(data)
}

func (r *RSI) calculateRSI(data *rsiData) {
	if !data.initialized {
		var sumGain, sumLoss float64
		for i := 1; i <= r.period; i++ {
			change := data.prices[i] - data.prices[i-1]
			if change > 0 {
				sumGain += change
			} else {
				sumLoss += -change
			}
		}
		data.avgGain = sumGain / float64(r.period)
		data.avgLoss = sumLoss / float64(r.period)
		data.initialized = true
	} else {
		change := data.prices[len(data.prices)-1] - data.prices[len(data.prices)-2]
		var gain, loss float64
		if change > 0 {
			gain = change
		} else {
			loss = -change
		}
		data.avgGain = (data.avgGain*float64(r.period-1) + gain) / float64(r.period)
		data.avgLoss = (data.avgLoss*float64(r.period-1) + loss) / float64(r.period)
	}

	if data.avgLoss == 0 {
		data.rsi = 100
	} else {
		rs := data.avgGain / data.avgLoss
		data.rsi = 100 - (100 / (1 + rs))
	}
}

func (r *RSI) Check(tokenID string) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	data, exists := r.data[tokenID]
	if !exists || !data.initialized {
		return false // Allow trading if no data yet
	}

	return data.rsi < r.overbought && data.rsi > r.oversold
}

func (r *RSI) IsReady(tokenID string) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	data, exists := r.data[tokenID]
	return exists && data.initialized
}
