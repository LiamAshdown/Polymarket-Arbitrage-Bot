package client

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

type EIP712OrderSigner struct {
	privateKey *ecdsa.PrivateKey
	chainID    int64
	verifier   string
}

func NewEIP712OrderSigner(privateKeyHex string, chainID int64, verifierContract string) (*EIP712OrderSigner, error) {
	if len(privateKeyHex) > 2 && privateKeyHex[:2] == "0x" {
		privateKeyHex = privateKeyHex[2:]
	}

	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	return &EIP712OrderSigner{
		privateKey: privateKey,
		chainID:    chainID,
		verifier:   verifierContract,
	}, nil
}

func (s *EIP712OrderSigner) SignOrder(params OrderSignParams) (*PolymarketOrder, error) {
	now := float64(time.Now().UTC().Unix())
	randomFloat := rand.Float64()
	saltFloat := now * randomFloat
	salt := big.NewInt(int64(saltFloat))

	priceWei := new(big.Float).Mul(big.NewFloat(params.Price), big.NewFloat(1e6))
	sizeWei := new(big.Float).Mul(big.NewFloat(params.Size), big.NewFloat(1e6))

	var makerAmount, takerAmount *big.Int
	if params.Side == "BUY" {
		totalCost := new(big.Float).Mul(priceWei, big.NewFloat(params.Size))
		makerAmount, _ = totalCost.Int(nil)
		takerAmount, _ = sizeWei.Int(nil)
	} else {
		makerAmount, _ = sizeWei.Int(nil)
		totalReceived := new(big.Float).Mul(priceWei, big.NewFloat(params.Size))
		takerAmount, _ = totalReceived.Int(nil)
	}

	typedData := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": []apitypes.Type{
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Order": []apitypes.Type{
				{Name: "salt", Type: "uint256"},
				{Name: "maker", Type: "address"},
				{Name: "signer", Type: "address"},
				{Name: "taker", Type: "address"},
				{Name: "tokenId", Type: "uint256"},
				{Name: "makerAmount", Type: "uint256"},
				{Name: "takerAmount", Type: "uint256"},
				{Name: "expiration", Type: "uint256"},
				{Name: "nonce", Type: "uint256"},
				{Name: "feeRateBps", Type: "uint256"},
				{Name: "side", Type: "uint8"},
				{Name: "signatureType", Type: "uint8"},
			},
		},
		PrimaryType: "Order",
		Domain: apitypes.TypedDataDomain{
			Name:              "Polymarket CTF Exchange",
			Version:           "1",
			ChainId:           math.NewHexOrDecimal256(s.chainID),
			VerifyingContract: s.verifier,
		},
		Message: apitypes.TypedDataMessage{
			"salt":          salt.String(),
			"maker":         strings.ToLower(params.Maker),
			"signer":        strings.ToLower(params.Signer),
			"taker":         strings.ToLower(params.Taker),
			"tokenId":       params.TokenID,
			"makerAmount":   makerAmount.String(),
			"takerAmount":   takerAmount.String(),
			"expiration":    strconv.FormatInt(params.Expiration, 10),
			"nonce":         params.Nonce,
			"feeRateBps":    params.FeeRateBps,
			"side":          strconv.Itoa(sideToInt(params.Side)),
			"signatureType": strconv.Itoa(params.SignatureType),
		},
	}

	// Hash and sign
	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		return nil, fmt.Errorf("failed to hash domain: %w", err)
	}

	messageHash, err := typedData.HashStruct(typedData.PrimaryType, typedData.Message)
	if err != nil {
		return nil, fmt.Errorf("failed to hash message: %w", err)
	}

	rawData := []byte{0x19, 0x01}
	rawData = append(rawData, domainSeparator...)
	rawData = append(rawData, messageHash...)
	digest := crypto.Keccak256Hash(rawData)
	fmt.Printf("  Final Digest: 0x%s\n", hex.EncodeToString(digest.Bytes()))

	signature, err := crypto.Sign(digest.Bytes(), s.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	signature[64] += 27

	saltUint64 := salt.Uint64()

	return &PolymarketOrder{
		Salt:          saltUint64,
		Maker:         params.Maker,
		Signer:        params.Signer,
		Taker:         params.Taker,
		TokenID:       params.TokenID,
		MakerAmount:   makerAmount.String(),
		TakerAmount:   takerAmount.String(),
		Expiration:    strconv.FormatInt(params.Expiration, 10),
		Nonce:         params.Nonce,
		FeeRateBps:    params.FeeRateBps,
		Side:          params.Side,
		SignatureType: params.SignatureType,
		Signature:     "0x" + hex.EncodeToString(signature),
	}, nil
}

func (s *EIP712OrderSigner) GetAddress() common.Address {
	return crypto.PubkeyToAddress(s.privateKey.PublicKey)
}

func sideToInt(side string) int {
	if side == "BUY" {
		return 0
	}
	return 1 // SELL
}
