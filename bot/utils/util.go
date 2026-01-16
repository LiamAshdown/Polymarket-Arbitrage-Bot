package utils

import (
	"crypto/rand"
	"encoding/hex"
	"math"
	"time"
)

func CalculateNearest15M() time.Time {
	now := time.Now()
	nowUnix := now.Unix()

	const interval int64 = 15 * 60
	floor := (nowUnix / interval) * interval
	ceil := floor + interval

	if nowUnix-floor <= ceil-nowUnix {
		return time.Unix(floor, 0)
	}

	return time.Unix(ceil, 0)
}

func GenerateRandomID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func AbsFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func RoundToTick(price, tickSize float64) float64 {
	if tickSize <= 0 {
		return price
	}
	return math.Round(price/tickSize) * tickSize
}
