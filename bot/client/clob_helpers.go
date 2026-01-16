package client

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

func CreateL1AuthSignature(signer *EIP712OrderSigner, timestamp int64, nonce int) (string, error) {
	address := signer.GetAddress().Hex()

	typedData := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": []apitypes.Type{
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
			},
			"ClobAuth": []apitypes.Type{
				{Name: "address", Type: "address"},
				{Name: "timestamp", Type: "string"},
				{Name: "nonce", Type: "uint256"},
				{Name: "message", Type: "string"},
			},
		},
		PrimaryType: "ClobAuth",
		Domain: apitypes.TypedDataDomain{
			Name:    "ClobAuthDomain",
			Version: "1",
			ChainId: math.NewHexOrDecimal256(signer.chainID),
		},
		Message: apitypes.TypedDataMessage{
			"address":   address,
			"timestamp": strconv.FormatInt(timestamp, 10),
			"nonce":     math.NewHexOrDecimal256(int64(nonce)),
			"message":   "This message attests that I control the given wallet",
		},
	}

	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		return "", fmt.Errorf("failed to hash domain: %w", err)
	}

	messageHash, err := typedData.HashStruct("ClobAuth", typedData.Message)
	if err != nil {
		return "", fmt.Errorf("failed to hash message: %w", err)
	}

	rawData := []byte(fmt.Sprintf("\x19\x01%s%s", string(domainSeparator), string(messageHash)))
	digest := crypto.Keccak256Hash(rawData)

	signature, err := crypto.Sign(digest.Bytes(), signer.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign: %w", err)
	}
	signature[64] += 27

	return "0x" + hex.EncodeToString(signature), nil
}

// GetTickSize fetches tick size and min order size for a token.
func (c *ClobClient) GetTickSizeAndMin(ctx context.Context, tokenId string) (tickSize float64, minSize float64, err error) {
	book, err := c.GetBook(ctx, tokenId)
	if err != nil {
		return 0, 0, err
	}

	tickSize = float64(book.TickSize)

	// Parse min order size
	if book.MinOrderSize != "" {
		minSize, err = strconv.ParseFloat(book.MinOrderSize, 64)
		if err != nil {
			return tickSize, 0, fmt.Errorf("failed to parse min order size: %w", err)
		}
	}

	return tickSize, minSize, nil
}
