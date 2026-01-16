package client

// OrderSigner creates signed Polymarket orders using EIP-712.
// This interface abstracts the complex EIP-712 signing logic.
// Reference: https://github.com/Polymarket/clob-client/blob/main/src/signing/eip712.ts
type OrderSigner interface {
	SignOrder(params OrderSignParams) (*PolymarketOrder, error)
}

type OrderSignParams struct {
	TokenID       string
	Side          string  // "BUY" or "SELL"
	Price         float64 // Decimal price (e.g., 0.65)
	Size          float64 // Quantity in shares
	Maker         string  // Funder address
	Signer        string  // Signing address (usually same as maker for EOA/GNOSIS_SAFE)
	Taker         string  // Taker address (operator, use zero address for open orders)
	Nonce         string  // Current exchange nonce for the maker
	FeeRateBps    string  // Fee rate in basis points
	Expiration    int64   // Unix timestamp expiration
	SignatureType int     // 0=EOA, 1=POLY_PROXY, 2=GNOSIS_SAFE
}
