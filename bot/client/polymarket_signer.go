package client

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type PolymarketL2Auth struct {
	Address    string // POLY_ADDRESS (signer/wallet address)
	APIKey     string // POLY_API_KEY
	Secret     string // base64 encoded secret for HMAC
	Passphrase string // POLY_PASSPHRASE
}

func (a PolymarketL2Auth) Apply(req *http.Request) error {
	return a.sign(req, "")
}

func (a PolymarketL2Auth) SignWithBody(req *http.Request, body string) error {
	return a.sign(req, body)
}

func (a PolymarketL2Auth) sign(req *http.Request, body string) error {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	method := req.Method
	path := req.URL.Path

	message := timestamp + method + path + body

	// Decode secret using URL-safe base64 (matches Python's urlsafe_b64decode)
	secretBytes, err := base64.URLEncoding.DecodeString(a.Secret)
	if err != nil {
		return fmt.Errorf("failed to decode secret: %w", err)
	}

	h := hmac.New(sha256.New, secretBytes)
	h.Write([]byte(message))

	// Encode signature using URL-safe base64 (matches Python's urlsafe_b64encode)
	sig := h.Sum(nil)
	signature := base64.URLEncoding.EncodeToString(sig)

	req.Header.Set("POLY_ADDRESS", a.Address)
	req.Header.Set("POLY_SIGNATURE", signature)
	req.Header.Set("POLY_TIMESTAMP", timestamp)
	req.Header.Set("POLY_API_KEY", a.APIKey)
	req.Header.Set("POLY_PASSPHRASE", a.Passphrase)

	return nil
}
