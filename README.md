# Polymarket Arbitrage Bot

This is an Arbitrage Bot for PolyMarket.

This bot focuses when the "YES" + "NO" < $1 and places orders as a maker but can immediantly hedge a position if not filled.

**Key Features**:

- Calculates the edge between "YES" and "NO" prices
- Checks hedge liquidity before placing any orders
- Cancel stale orders if they remain unfilled
- Handles partial fills by hedging the remaining quantity.

The bot supports both "LIVE" and "PAPER" modes. The "PAPER" mode mimics the live market as closely as possible, using real-time order book data to simulate partial and full fills.

Designed with performance in mind, this bot aims to execute orders the moment an edge is detected.

## Installation

You will need `golang` with the minimum version of `1.24.0`.

There's an `.env.example` that you will need to copy and fill out the environment variables.

And then you're ready to run the bot.

**Note:** Even though there's support for running on Live - I've not yet attempted this due to Polymarket being blocked in my region. If you like to help out to test this, please open an issue.