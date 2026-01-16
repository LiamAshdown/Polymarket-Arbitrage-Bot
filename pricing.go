package bot

import (
	"quadbot/utils"
)

func makerBuyPrice(bid, ask, tick float64) float64 {
	// Step inside spread if possible; must remain < ask to stay maker.
	// If spread is wide (>=2 ticks), place at ask-1tick to jump the queue.
	// Example: bid=$0.48, ask=$0.52 -> place at $0.51 (front of queue, faster fills)
	// If spread is narrow (<2 ticks), join existing bid (back of queue, slower fills)
	// Tradeoff: Pay slightly more for better queue position while remaining maker.
	if tick > 0 && (ask-bid) >= 2*tick {
		return utils.RoundToTick(ask-tick, tick)
	}
	return utils.RoundToTick(bid, tick)
}

func makerSellPrice(bid, ask, tick float64) float64 {
	if tick > 0 && (ask-bid) >= 2*tick {
		return utils.RoundToTick(bid+tick, tick)
	}
	return utils.RoundToTick(ask, tick)
}

func EdgeNet(side string, upPrice, downPrice, qty, buffer float64, feeRateBps int, upIsTaker, downIsTaker bool) (edgeNet float64, feePerPairShare float64) {
	var feeUp, feeDn float64

	if upIsTaker {
		feeUp = utils.CalculateTakerFeeUSDC(upPrice, qty, feeRateBps)
	}

	if downIsTaker {
		feeDn = utils.CalculateTakerFeeUSDC(downPrice, qty, feeRateBps)
	}

	feePerPairShare = (feeUp + feeDn) / qty

	sum := upPrice + downPrice
	if side == "BUY" {
		edgeNet = 1.00 - sum - feePerPairShare - buffer
	} else { // SELL
		edgeNet = sum - 1.00 - feePerPairShare - buffer
	}
	return edgeNet, feePerPairShare
}

func EdgeBuyNet(upPrice, downPrice, qty, buffer float64, feeRateBps int, upIsTaker, downIsTaker bool) (edgeNet float64, feePerPairShare float64) {
	return EdgeNet("BUY", upPrice, downPrice, qty, buffer, feeRateBps, upIsTaker, downIsTaker)
}

func EdgeSellNet(upPrice, downPrice, qty, buffer float64, feeRateBps int, upIsTaker, downIsTaker bool) (edgeNet float64, feePerPairShare float64) {
	return EdgeNet("SELL", upPrice, downPrice, qty, buffer, feeRateBps, upIsTaker, downIsTaker)
}

type CompletedPosition struct {
	Quantity float64
	Cost     float64
	Profit   float64
}

func calculateBinaryPnL(price1 float64, price2 float64, quantity float64) CompletedPosition {
	totalCost := (price1 + price2) * quantity
	expectedPayout := quantity * binaryMarketPayout
	profit := expectedPayout - totalCost

	return CompletedPosition{
		Quantity: quantity,
		Cost:     totalCost,
		Profit:   profit,
	}
}
