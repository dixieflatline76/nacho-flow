# Change Log

All notable changes to the Nacho Flow VS Code Extension will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.6.0] - 2026-08-23

### Added
- Initial release of Nacho Flow VS Code Extension
- Status bar item showing real-time cost savings and routing statistics
- Basic REST API client for communicating with Nacho Flow daemon
- SSE client for real-time event streaming
- Authentication token management using VS Code SecretStorage
- Core extension architecture and command registration

### Features
- Real-time monitoring of routing decisions and cost savings
- Connection to Nacho Flow daemon via HTTP REST and SSE
- Secure storage of authentication tokens
- Basic dashboard with status information

## [Unreleased]

### Planned
- Webview dashboard panels for route history and circuit breakers
- Interactive configuration editor with validation
- Circuit breaker reset functionality
- Auto-tuning advisor integration
- Enhanced visualization and reporting