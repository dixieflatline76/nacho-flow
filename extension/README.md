# Nacho Flow VS Code Extension

A lightweight, high-visibility UI companion for the `nacho-flow` AI routing gateway. Provides real-time cost visibility, route inspection, circuit breaker management, and interactive configuration editing directly inside VS Code.

## Features

- **Real-time Status Monitoring**: See cost savings and routing statistics in the status bar
- **Route History Dashboard**: View recent prompt routing decisions with latency and cost information
- **Circuit Breaker Management**: Monitor provider health and reset tripped circuits
- **Configuration Editor**: Edit and validate routing rules with dry-run testing
- **Auto-Tuning Advisor**: Get recommendations for optimizing your routing configuration

## Requirements

- Nacho Flow daemon running (v0.6.0 or later)
- VS Code 1.80.0 or later

## Installation

1. Install the Nacho Flow VS Code Extension from the marketplace
2. Ensure the Nacho Flow daemon is running (default: `http://127.0.0.1:8000`)
3. Configure your authentication token in VS Code settings

## Configuration

The extension can be configured through VS Code settings:

- `nachoFlow.daemonUrl`: URL of the Nacho Flow daemon (default: `http://127.0.0.1:8000`)

## Usage

- Open the Nacho Flow Dashboard using the command palette (`Cmd+Shift+P` / `Ctrl+Shift+P`) and search for "Nacho Flow: Show Dashboard"
- View real-time statistics in the status bar
- Reset circuit breakers when needed
- Edit and optimize your routing configuration

## Architecture

This extension follows the "Thin-Client Doctrine":

1. **Zero Core Logic in TypeScript**: All domain logic resides in the Go daemon
2. **Pure Presentation & Control Layer**: Communicates with the daemon over HTTP REST and SSE streams
3. **No Mandatory Local Binary Bundling**: Connects to a running daemon (local or remote)

## Contributing

This extension is part of the Nacho Flow project. Contributions are welcome!

## License

MIT