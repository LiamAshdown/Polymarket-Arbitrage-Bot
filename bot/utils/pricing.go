package utils

import "math"

type feePoint struct {
	Price  float64
	Fee100 float64 // fee in USDC for 100 shares at this price (taker)
}

var feeTable100 = []feePoint{
	{0.01, 0.0000},
	{0.05, 0.0030},
	{0.10, 0.0200},
	{0.15, 0.0600},
	{0.20, 0.1300},
	{0.25, 0.2200},
	{0.30, 0.3300},
	{0.35, 0.4500},
	{0.40, 0.5800},
	{0.45, 0.6900},
	{0.50, 0.7800},
	{0.55, 0.8400},
	{0.60, 0.8600},
	{0.65, 0.8400},
	{0.70, 0.7700},
	{0.75, 0.6600},
	{0.80, 0.5100},
	{0.85, 0.3500},
	{0.90, 0.1800},
	{0.95, 0.0500},
	{0.99, 0.0000},
}

func feePer100Shares(price float64) float64 {
	if price <= feeTable100[0].Price {
		return feeTable100[0].Fee100
	}
	last := feeTable100[len(feeTable100)-1]
	if price >= last.Price {
		return last.Fee100
	}
	for i := 0; i < len(feeTable100)-1; i++ {
		a := feeTable100[i]
		b := feeTable100[i+1]
		if price >= a.Price && price <= b.Price {
			t := (price - a.Price) / (b.Price - a.Price)
			return a.Fee100 + t*(b.Fee100-a.Fee100)
		}
	}
	return last.Fee100
}

func CalculateTakerFeeUSDC(price, quantity float64, feeRateBps int) float64 {
	if feeRateBps <= 0 {
		return 0
	}
	scale := float64(feeRateBps) / 1000.0
	fee100 := feePer100Shares(price) * scale
	fee := (fee100 / 100.0) * quantity
	fee = math.Round(fee*1e4) / 1e4
	if fee > 0 && fee < 0.0001 {
		fee = 0.0001
	}
	return fee
}

// CalculateMakerRebateUSDC calculates the rebate received for maker orders
// Makers typically receive a 2 bps rebate (0.02% of notional value)
func CalculateMakerRebateUSDC(price, quantity float64, rebateRateBps int) float64 {
	if rebateRateBps <= 0 {
		return 0
	}
	// Rebate is calculated on notional value (price * quantity)
	notional := price * quantity
	rebate := notional * (float64(rebateRateBps) / 10000.0)
	rebate = math.Round(rebate*1e4) / 1e4
	return rebate
}
