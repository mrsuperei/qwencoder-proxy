# Task 001: Create Proxy Configuration Data Structures

## Description
Create the foundational data structures for proxy configuration that will be used throughout the application. This task involves defining the core types for proxy settings, health tracking, and cache keys that enable per-token proxy configuration support.

## Technical Context
This task is part of the Foundation Phase (Phase 1) of the proxy-per-token architecture implementation. It draws upon:

- **SOLID Principles**: Following the Single Responsibility Principle by creating focused data structures
- **Dependency Inversion Principle**: Creating abstractions that will be used throughout the codebase
- **Design Patterns**: Value object pattern for immutable configuration data

**Related Components**:
- [`auth/multi_token_store.go`](auth/multi_token_store.go) - Will use these structures to extend ProviderToken
- [`config/proxy_client_factory.go`](config/proxy_client_factory.go) - Will use ProxyConfig for client creation
- [`auth/proxy_health_tracker.go`](auth/proxy_health_tracker.go) - Will use ProxyHealth for tracking

**Architectural Principles**:
- Backward compatibility: New structures must work with existing tokens
- Security: Proxy credentials must be handled securely
- Validation: All proxy configurations must be validated before use

## Subtasks
1. Create `auth/proxy_config.go` file with ProxyType enum (none, http, https, socks5)
2. Define ProxyConfig struct with fields: Type, Host, Port, Username, Password, Enabled
3. Define ProxyHealth struct with fields: LastCheck, IsHealthy, LastError, ConsecutiveFailures, AverageLatencyMs
4. Define ProxyConfigKey struct for client cache key generation
5. Implement String() method on ProxyConfigKey for cache map usage
6. Implement Validate() method on ProxyConfig to validate configuration
7. Implement Validate() method on ProxyType to ensure valid proxy type
8. Add JSON marshaling/unmarshaling with omitempty tags for backward compatibility
9. Add masking functionality for passwords in log output
10. Add unit tests for all validation methods

## Dependencies
- None (foundational task)

## Files to Create
- `auth/proxy_config.go` (new file)
- `auth/proxy_config_test.go` (new test file)
