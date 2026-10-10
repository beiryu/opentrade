// Package rules encodes the trading rules of the Vietnamese stock market:
// board lots, tick sizes, daily price bands, T+2 settlement and transaction
// costs. Everything the engine needs to refuse an order the exchange would
// refuse, before a model's decision ever reaches a broker.
//
// Prices are in VND (not thousands). Sources: HOSE/HNX trading regulations
// as of 2025 (KRX system), Circular 120/2020/TT-BTC for settlement, and the
// 0.1 % personal income tax on sale proceeds.
package rules

import (
	"fmt"
	"math"
)

// Exchange is one of the three Vietnamese equity boards.
type Exchange string

const (
	HOSE  Exchange = "HOSE"
	HNX   Exchange = "HNX"
	UPCOM Exchange = "UPCOM"
)

// Lot is the board lot: orders on the continuous book are multiples of 100
// shares. Odd lots trade on a separate book that OpenTrade does not model.
const Lot = 100

// SettlementDays is T+2: shares bought on trading day D are in the account
// and sellable from trading day D+2; cash from a sale is likewise available
// on D+2.
const SettlementDays = 2

// FeeRate is the brokerage fee assumed on both buys and sells (0.15 %), a
// mid-range retail rate. SellTaxRate is the personal income tax withheld on
// the gross proceeds of every sale (0.1 %).
const (
	FeeRate     = 0.0015
	SellTaxRate = 0.001
)

// ParseExchange accepts the three board names, case-insensitively.
func ParseExchange(s string) (Exchange, error) {
	switch Exchange(upper(s)) {
	case HOSE:
		return HOSE, nil
	case HNX:
		return HNX, nil
	case UPCOM:
		return UPCOM, nil
	}
	return "", fmt.Errorf("rules: unknown exchange %q", s)
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'a' <= c && c <= 'z' {
			b[i] = c - 'a' + 'A'
		}
	}
	return string(b)
}

// BandPct is the daily price band around the reference price: ±7 % on HOSE,
// ±10 % on HNX, ±15 % on UPCOM.
func BandPct(ex Exchange) float64 {
	switch ex {
	case HNX:
		return 0.10
	case UPCOM:
		return 0.15
	default:
		return 0.07
	}
}

// TickSize is the minimum price increment. HOSE steps 10 VND under 10,000,
// 50 VND from 10,000 to under 50,000 and 100 VND from 50,000. HNX and UPCOM
// use 100 VND at every price.
func TickSize(ex Exchange, price float64) float64 {
	if ex == HNX || ex == UPCOM {
		return 100
	}
	switch {
	case price < 10_000:
		return 10
	case price < 50_000:
		return 50
	default:
		return 100
	}
}

// RoundToTick rounds a price to the nearest valid tick for the exchange.
func RoundToTick(ex Exchange, price float64) float64 {
	tick := TickSize(ex, price)
	return math.Round(price/tick) * tick
}

// IsOnTick reports whether a price sits exactly on a tick.
func IsOnTick(ex Exchange, price float64) bool {
	tick := TickSize(ex, price)
	q := price / tick
	return math.Abs(q-math.Round(q)) < 1e-6
}

// Band returns the floor and ceiling prices for a trading day given the
// reference price (the previous close). The ceiling is rounded down to a
// tick and the floor rounded up, so both stay inside the band; this is how
// the exchanges publish them.
func Band(ex Exchange, ref float64) (floor, ceiling float64) {
	pct := BandPct(ex)
	hi := ref * (1 + pct)
	lo := ref * (1 - pct)
	ceiling = math.Floor(hi/TickSize(ex, hi)) * TickSize(ex, hi)
	floor = math.Ceil(lo/TickSize(ex, lo)) * TickSize(ex, lo)
	return floor, ceiling
}

// InBand reports whether a price is between floor and ceiling inclusive.
func InBand(ex Exchange, ref, price float64) bool {
	floor, ceiling := Band(ex, ref)
	return price >= floor-1e-9 && price <= ceiling+1e-9
}

// IsLotMultiple reports whether a share quantity is a whole number of lots.
func IsLotMultiple(qty int) bool {
	return qty > 0 && qty%Lot == 0
}

// BuyCost is the cash leaving the account for a buy of the given notional
// (price × quantity): the notional plus the fee.
func BuyCost(notional float64) float64 {
	return notional * (1 + FeeRate)
}

// SellProceeds is the cash arriving for a sale of the given notional: the
// notional minus fee and sale tax.
func SellProceeds(notional float64) float64 {
	return notional * (1 - FeeRate - SellTaxRate)
}

// MaxAffordableLots is the largest whole number of lots that BuyCost fits in
// the given cash at the given price. Zero when even one lot is too much.
func MaxAffordableLots(cash, price float64) int {
	if price <= 0 || cash <= 0 {
		return 0
	}
	perLot := BuyCost(price * Lot)
	return int(math.Floor(cash / perLot))
}
