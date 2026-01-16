package client

import (
	"net/http"
	"quadbot/logger"
	"time"

	"github.com/gorilla/websocket"
)

// WSClient provides shared websocket utilities (headers, connect, read/write, close)
// used by specialized clients like WSMarketClient and WSUserClient.
type WSClient struct {
	conn         *websocket.Conn
	url          string
	headers      map[string]string
	logger       logger.Logger
	pingInterval time.Duration
	stopPing     chan struct{}
}

func NewWSClient(url string, logger logger.Logger) *WSClient {
	return &WSClient{
		url:          url,
		headers:      map[string]string{},
		logger:       logger,
		pingInterval: 50 * time.Second,
		stopPing:     make(chan struct{}),
	}
}

func (ws *WSClient) SetHeaders(h map[string]string) {
	if ws.headers == nil {
		ws.headers = map[string]string{}
	}
	for k, v := range h {
		ws.headers[k] = v
	}
}

func (ws *WSClient) Connect() error {
	reqHeaders := make(http.Header)
	for k, v := range ws.headers {
		reqHeaders.Set(k, v)
		ws.logger.Info("ws_header", "key", k, "value", v)
	}
	conn, resp, err := websocket.DefaultDialer.Dial(ws.url, reqHeaders)
	if err != nil {
		if resp != nil {
			ws.logger.Error("ws_connect_failed", "status", resp.Status, "err", err)
		}
		return err
	}
	ws.conn = conn
	ws.logger.Info("ws_connected", "url", ws.url)

	go ws.startPinger()

	return nil
}

func (ws *WSClient) Close() error {
	close(ws.stopPing)
	if ws.conn != nil {
		return ws.conn.Close()
	}
	return nil
}

func (ws *WSClient) WriteJSON(v any) error {
	if ws.conn == nil {
		return websocket.ErrBadHandshake
	}
	return ws.conn.WriteJSON(v)
}

func (ws *WSClient) ReadMessage() (int, []byte, error) {
	if ws.conn == nil {
		return 0, nil, websocket.ErrBadHandshake
	}
	return ws.conn.ReadMessage()
}

func (ws *WSClient) startPinger() {
	ticker := time.NewTicker(ws.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ws.stopPing:
			return
		case <-ticker.C:
			if err := ws.conn.WriteMessage(websocket.TextMessage, []byte("PING")); err != nil {
				ws.logger.Error("ping_failed", "err", err)
				return
			}
		}
	}
}
