package main

import (
	"context"
	"fmt"
	_ "net/http/pprof"
	"os"
	"quadbot/bot"
	"quadbot/client"
	"quadbot/engine"
	"quadbot/logger"
	service "quadbot/services"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	logger := logger.NewLogger()

	// Load .env file if it exists (optional)
	_ = godotenv.Load()

	// Profiling for testing performance..
	// go func() {
	// 	logger.Info("pprof_server_started", "url", "http://localhost:6060/debug/pprof")
	// 	if err := http.ListenAndServe("localhost:6060", nil); err != nil {
	// 		logger.Error("pprof_server_failed", "error", err)
	// 	}
	// }()

	engineMode := os.Getenv("ENGINE_MODE")
	if engineMode == "" {
		engineMode = "paper"
	}

	engineMode = "paper"

	gammaClient := client.NewGammaClient()
	clobClient := client.NewClobClient()
	apiClient := client.NewAPIClient()

	ctx := context.Background()

	logger.Info("testing_latency", "test", "Testing latency to Polymarket...")

	latency, err := clobClient.GetAverageLatency(ctx)

	if err != nil {
		logger.Error("failed_to_get_latency", "err", err)
		return
	}

	logger.Info("avg_latency", "latency", latency)

	if latency > 600*time.Millisecond {
		logger.Warn("latency_high", "reason", fmt.Sprintf("Your latency is high (%s) - this can affect your profit as execution is slower. Maybe move closer to Polymarket?", latency))
	}

	eventFinder := service.NewEventFinder(gammaClient)
	event, err := eventFinder.Find15METHEvent(ctx)

	if err != nil {
		logger.Error("event_not_found", "error", err)
		return
	}

	if !event.IsActive() {
		logger.Error("event_inactive", "event", event.MarketEvent.Active)
		return
	}

	logger.Info("event_found", "event", event.MarketEvent.ID)

	var tradingEngine engine.ExecutionEngine

	if engineMode == "live" {
		logger.Info("Starting LIVE trading engine - REAL MONEY")
		logger.Info("Checking if we're not geoblocked")

		geoBlock, err := apiClient.GetGeoBlock(ctx)

		if err != nil {
			logger.Error("failed_to_get_geoblock", "error", err)
			return
		}

		if geoBlock.Blocked {
			logger.Error("geoblocked", "msg", "Couldn't start engine. The bot is geoblocked. Please ensure the bot is running in a region that isn't blocked by Polymarket")
			return
		}

		logger.Info("geoblocked", "msg", "We're not geoblocked!")

		privateKey := os.Getenv("PRIVATE_KEY")
		if privateKey == "" {
			logger.Error("missing_config", "msg", "PRIVATE_KEY environment variable is required for live trading")
			return
		}

		apiKey, err := clobClient.CreateOrDeriveApiKey(ctx, privateKey)

		if err != nil {
			logger.Error("failed_to_get_api_key", "error", err)
			return
		}

		signer, err := client.NewEIP712OrderSigner(
			privateKey,
			137,
			"0x4bFb41d5B3570DeFd03C39a9A4D8dE6Bd8B8982E",
		)
		if err != nil {
			logger.Error("Failed to create order signer", "error", err)
			return
		}

		walletAddress := signer.GetAddress().Hex()

		cfg := engine.LiveEngineConfig{
			APIAddress:    walletAddress,
			APIKey:        apiKey.ApiKey,
			APISecret:     apiKey.Secret,
			APIPassphrase: apiKey.Passphrase,
		}

		liveEngine := engine.NewLiveEngine(cfg, signer, *event, *logger)
		tradingEngine = liveEngine
	} else {
		logger.Info("Starting PAPER trading engine (simulation)")
		tradingEngine = engine.NewPaperEngine(*logger)
	}

	tradingBot := bot.NewBot(tradingEngine, event, bot.BotConfig{
		BufferTicks:     getEnvFloat("BUFFER_TICKS", 0),
		MinEdge:         getEnvFloat("MIN_EDGE", 0.01),
		OrderTimeout:    time.Duration(getEnvInt("ORDER_TIMEOUT_SECONDS", 30)) * time.Second,
		MaxPairs:        getEnvInt("MAX_PAIRS", 1),
		MinQuantity:     getEnvFloat("MIN_QUANTITY", 5.0),
		MaxQuantity:     getEnvFloat("MAX_QUANTITY", 5.0),
		MaxSpreadBps:    getEnvFloat("MAX_SPREAD_BPS", 12500.0),
		ScaleWithSpread: getEnvBool("SCALE_WITH_SPREAD", true),
	}, *logger)

	if err := tradingBot.Run(ctx); err != nil {
		panic(err)
	}
}

func getEnvInt(key string, defaultValue int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseFloat(val, 64); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseBool(val); err == nil {
			return parsed
		}
	}
	return defaultValue
}
