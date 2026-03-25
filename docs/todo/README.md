# Qwencoder Proxy - Implementation Plans

This directory contains detailed implementation plans for architectural improvements and bug fixes identified in the comprehensive architectural audit.

## Overview

Each plan file provides:
- **Problem Description:** Clear explanation of the issue
- **Current State:** Analysis of existing code
- **Solution Architecture:** Proposed solution design
- **Implementation Plan:** Step-by-step implementation guide
- **Testing Strategy:** Unit and integration test plans
- **Verification Checklist:** Progress tracking
- **Impact Analysis:** Benefits, risks, and side effects

## Implementation Plans

### 01. Fix OAuth Email Extraction Bug
**Priority:** CRITICAL  
**Estimated Time:** 5 minutes  
**Complexity:** Low

**Problem:** Emails are being saved as `unknown@example.com` instead of actual user email during OAuth flow.

**Root Cause:** `extractEmailFromToken()` in [`rest_api.go:1178`](../rest_api/rest_api.go:1178) passes `nil` as `tokenResponse` parameter instead of the actual token response map.

**Solution:** Modify function signature to accept `tokenResponse` parameter and pass it from `saveCredentials()`.

**Impact:** Immediate fix for email extraction across all OAuth providers.

---

### 02. Implement Per-Token Rate Limiting
**Priority:** CRITICAL  
**Estimated Time:** 2 weeks  
**Complexity:** High

**Problem:** No rate limiting mechanism exists, which is critical for managing API quotas and preventing abuse.

**Solution Architecture:**
- New `internal/ratelimit` package with RateLimiter interface
- HTTP middleware for rate limiting
- Database schema for token usage tracking
- Integration with token selection

**Key Components:**
- Rate limiting strategies (token-based, request-based, hybrid)
- Token usage tracking
- Rate limit hooks in request flow

**Impact:** Enables granular per-token rate limiting with configurable strategies.

---

### 03. Refactor Handler Constructors Using Functional Options
**Priority:** HIGH  
**Estimated Time:** 2 days  
**Complexity:** Medium

**Problem:** 21 duplicate constructor functions across handler types with repetitive code.

**Solution:** Replace multiple constructors with single constructor using functional options pattern.

**Impact:** Reduces constructor count from 21 to 5 (76% code reduction).

---

### 04. Centralize Error Handling
**Priority:** MEDIUM  
**Estimated Time:** 2 days  
**Complexity:** Low

**Problem:** Error handling is scattered with repeated patterns and inconsistent error context.

**Solution:** Create dedicated error package with standardized error types, codes, and context.

**Key Components:**
- `internal/errors/errors.go` - Error types and codes
- `internal/errors/http_handler.go` - HTTP error handlers
- Consistent error wrapping with context

**Impact:** Better error messages, easier debugging, standardized error codes.

---

### 05. Break Down MultiTokenManager
**Priority:** MEDIUM  
**Estimated Time:** 3 days  
**Complexity:** High

**Problem:** `MultiTokenManager` is a god object managing too many responsibilities (7 distinct concerns).

**Solution:** Split into focused, testable components:
- Manager package (token management)
- Refresh package (refresh scheduling)
- Health package (health tracking)
- Proxy health package (proxy health tracking)

**Impact:** Better separation of concerns, easier testing, improved maintainability.

---

### 06. Implement Provider Interface Extensions
**Priority:** MEDIUM  
**Estimated Time:** 2 days  
**Complexity:** Medium

**Problem:** Providers lack common interfaces for operations like token refresh, health checks, and proxy configuration.

**Solution:** Define interfaces for:
- `RefreshableProvider` - Token refresh capability
- `HealthCheckableProvider` - Health check capability
- `ProxyConfigurableProvider` - Proxy configuration capability
- `CapabilityProvider` - Capability reporting

**Impact:** Clear provider capabilities, easier testing, better abstraction.

---

### 07. Remove File-Based Storage
**Priority:** LOW  
**Estimated Time:** 1 day  
**Complexity:** Low

**Problem:** Dual storage backends (file + SQLite) create complexity and migration burden.

**Solution:** Remove all file-based storage code after SQLite migration is complete.

**Impact:** Simplified codebase, eliminated ~1000 lines of legacy code.

---

### 08. Extract OAuth Configuration
**Priority:** LOW  
**Estimated Time:** 1 day  
**Complexity:** Low

**Problem:** OAuth credentials (client IDs, secrets) are hardcoded in source code.

**Solution:** Move all OAuth credentials to configuration (environment variables or config file).

**Impact:** Better security, easier credential rotation, support for multiple environments.

---

### 09. Implement Request Context Tracking
**Priority:** LOW  
**Estimated Time:** 1 day  
**Complexity:** Low

**Problem:** Request context (token ID, provider ID, request ID) is not consistently tracked.

**Solution:** Create dedicated context package with:
- Standard context keys
- Request ID generation
- Token ID management
- Provider ID management
- Context propagation helpers

**Impact:** Better request tracing, improved debugging, consistent context usage.

---

## Implementation Order

### Phase 1: Critical Fixes (Week 1)
1. **Fix OAuth Email Extraction Bug** - 5 minutes
   - Modify [`rest_api.go:1166-1184`](../rest_api/rest_api.go:1166-1184)
   - Test all OAuth providers
   - Verify email extraction

### Phase 2: Code Consolidation (Weeks 2-3)
2. **Refactor Handler Constructors** - 2 days
   - Create `internal/proxy/handler_options.go`
   - Update all handler constructors
   - Update route registration
   - Write tests

3. **Centralize Error Handling** - 2 days
   - Create `internal/errors/` package
   - Update all error handling
   - Write tests

### Phase 3: Architecture Improvements (Weeks 4-5)
4. **Break Down MultiTokenManager** - 3 days
   - Create manager package
   - Create refresh package
   - Create health package
   - Create proxy health package
   - Update integration

5. **Implement Provider Interfaces** - 2 days
   - Create `internal/provider/interfaces.go`
   - Update all providers
   - Write tests

6. **Implement Request Context** - 1 day
   - Create `internal/context/` package
   - Update all handlers
   - Update middleware
   - Write tests

### Phase 4: Rate Limiting (Weeks 6-7)
7. **Implement Rate Limiting** - 2 weeks
   - Create `internal/ratelimit/` package
   - Add database schema
   - Implement middleware
   - Update token selection
   - Write tests

### Phase 5: Configuration & Cleanup (Week 8)
8. **Extract OAuth Config** - 1 day
   - Create `internal/config/oauth_config.go`
   - Update all OAuth credential usage
   - Write tests

9. **Remove File-Based Storage** - 1 day
   - Delete file store code
   - Remove migration logic
   - Update configuration

---

## Total Estimated Effort

| Phase | Duration | Complexity | Risk |
|--------|-----------|------------|-------|
| Critical Fixes | 5 minutes | Low | Low |
| Code Consolidation | 4 days | Medium | Low |
| Architecture Improvements | 6 days | High | Medium |
| Rate Limiting | 2 weeks | High | Medium |
| Configuration & Cleanup | 2 days | Low | Low |
| **Total** | **8 weeks** | - | **Medium** |

---

## Code Volume Impact

| Plan | Lines Added | Lines Removed | Net Change |
|-------|-------------|---------------|------------|
| Fix OAuth Email Extraction | ~10 | 0 | +10 |
| Refactor Handler Constructors | ~100 | ~80 | +20 |
| Centralize Error Handling | ~300 | ~200 | +100 |
| Break Down MultiTokenManager | ~800 | ~500 | +300 |
| Implement Provider Interfaces | ~150 | 0 | +150 |
| Implement Request Context | ~380 | 0 | +380 |
| Implement Rate Limiting | ~600 | 0 | +600 |
| Extract OAuth Config | ~200 | ~50 | +150 |
| Remove File-Based Storage | 0 | ~1000 | -1000 |
| **Total** | **~2540** | **~1830** | **+710** |

**Net Result:** +710 lines of better-organized, more maintainable code

---

## Testing Strategy

### Unit Tests
Each plan includes comprehensive unit tests:
- Test all new functions and methods
- Test edge cases and error conditions
- Test backward compatibility
- Achieve >80% code coverage

### Integration Tests
- Test component integration
- Test request flows end-to-end
- Test with real providers where possible
- Performance testing for rate limiting

### Manual Testing
- OAuth flows for all providers
- Rate limiting behavior
- Request context propagation
- Error handling and responses

---

## Risk Mitigation

### Low Risk Plans
1. **Backward Compatibility:**
   - Maintain existing API where possible
   - Provide migration paths for breaking changes
   - Document deprecation timeline

2. **Gradual Rollout:**
   - Implement changes incrementally
   - Test in staging before production
   - Monitor for issues after deployment

3. **Rollback Plan:**
   - Git branches for each phase
   - Clear rollback procedures
   - Database backups before schema changes
   - Feature flags for gradual rollout

### Medium Risk Plans
1. **Architecture Changes:**
   - Thorough code review before implementation
   - Pair programming for complex changes
   - Extensive testing of new components

2. **Performance Impact:**
   - Benchmark before and after changes
   - Monitor resource usage
   - Optimize database queries
   - Profile critical paths

---

## Success Criteria

Each plan is considered complete when:

- [ ] All code changes implemented
- [ ] All tests passing
- [ ] Code reviewed and approved
- [ ] Documentation updated
- [ ] Manual testing successful
- [ ] No regressions introduced
- [ ] Performance acceptable

---

## Next Steps

1. **Prioritize by Business Value:**
   - Fix OAuth email extraction (immediate user impact)
   - Implement rate limiting (critical for production)
   - Code consolidation (maintainability)

2. **Assign to Developers:**
   - Each phase to appropriate developer(s)
   - Code review process
   - Testing responsibility

3. **Track Progress:**
   - Weekly progress updates
   - Blocker identification
   - Risk escalation process

4. **Quality Gates:**
   - Code review required before merge
   - Test coverage minimum threshold
   - Performance benchmarks
   - Security review

---

## Additional Resources

- [Main Architecture Documentation](../architecture/README.md)
- [API Reference](../api-reference.md)
- [Authentication Guide](../authentication.md)
- [Migration Plan](../migration/comprehensive-sqlite-migration-plan.md)
- [Bug Reports](../bugs/)

---

**Last Updated:** 2026-03-16  
**Version:** 1.0
