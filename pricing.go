package bot

import (
	"math"
	"quadbot/utils"
)

func roundToTick(price, tick float64) float64 {
	if tick <= 0 {
		return price
	}
	rounded := math.Round(price/tick) * tick
	return math.Round(rounded*100) / 100
}

func makerBuyPrice(bid, ask, tick float64) float64 {
	// Step inside spread if possible; must remain < ask to stay maker.
	// If spread is wide (>=2 ticks), place at ask-1tick to jump the queue.
	// Example: bid=$0.48, ask=$0.52 -> place at $0.51 (front of queue, faster fills)
	// If spread is narrow (<2 ticks), join existing bid (back of queue, slower fills)
	// Tradeoff: Pay slightly more for better queue position while remaining maker.
	if tick > 0 && (ask-bid) >= 2*tick {
		return roundToTick(ask-tick, tick)
	}
	return roundToTick(bid, tick)
}

func makerSellPrice(bid, ask, tick float64) float64 {
	if tick > 0 && (ask-bid) >= 2*tick {
		return roundToTick(bid+tick, tick)
	}
	return roundToTick(ask, tick)
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
