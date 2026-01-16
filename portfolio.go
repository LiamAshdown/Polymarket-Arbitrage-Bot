package bot

import "sync"

type Portfolio struct {
	mu        sync.RWMutex
	initial   float64
	available float64
	reserved  map[string]float64 // orderID -> amount
	spent     float64
}

func NewPortfolio(starting float64) *Portfolio {

	return &Portfolio{
		initial:   starting,
		available: starting,
		reserved:  make(map[string]float64),
	}
}

func (p *Portfolio) Reserve(orderID string, amount float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.available -= amount
	p.reserved[orderID] += amount
}

func (p *Portfolio) Release(orderID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	amt := p.reserved[orderID]
	if amt > 0 {
		p.available += amt
		delete(p.reserved, orderID)
	}
}

func (p *Portfolio) Spend(amount float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.available -= amount
	p.spent += amount
}

func (p *Portfolio) Fill(orderID string, cost float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	reservedAmt := p.reserved[orderID]
	if reservedAmt > 0 {
		if reservedAmt >= cost {
			p.reserved[orderID] = reservedAmt - cost
			if p.reserved[orderID] == 0 {
				delete(p.reserved, orderID)
			}
		} else {
			remainder := cost - reservedAmt
			delete(p.reserved, orderID)
			p.available -= remainder
		}
	} else {
		p.available -= cost
	}
	p.spent += cost
}

func (p *Portfolio) Balances() (available float64, reservedTotal float64, spent float64, initial float64) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, v := range p.reserved {
		reservedTotal += v
	}
	return p.available, reservedTotal, p.spent, p.initial
}

func (p *Portfolio) HasAvailable(amount float64) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.available >= amount
}
