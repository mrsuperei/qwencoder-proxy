# Restructuring Migration Plan

## Overview

This document provides a detailed, step-by-step migration plan to restructure the `qwencoder-proxy` project according to Go best practices.

## Pre-Migration Checklist

- [ ] Create a backup of the current codebase
- [ ] Ensure all tests pass: `go test ./...`
- [ ] Commit any uncommitted changes
- [ ] Create a new branch for the migration: `git checkout -b restructure/cleanup-project-structure`
- [ ] Notify team members about the upcoming changes

## Phase 1: Documentation Reorganization (Low Risk)

### Objective
Move project management files to appropriate documentation locations.

### Steps

#### 1.1 Create Documentation Subdirectories

```bash
cd qwencoder-proxy
mkdir -p docs/plans docs/todo docs/bugs docs/architecture
```

#### 1.2 Move Project Management Files

```bash
# Move plans
mv plans/* docs/plans/
rmdir plans

# Move todos
mv todo/* docs/todo/
rmdir todo

# Move bugs
mv bugs/* docs/bugs/
rmdir bugs
```

#### 1.3 Verify Changes

```bash
# Check structure
ls -la docs/
ls -la docs/plans/
ls -la docs/todo/
ls -la docs/bugs/
```

#### 1.4 Update Documentation References

Search for references to moved files and update:
- README.md
- Any documentation that links to these files

### Import Path Changes
None (documentation only)

### Testing
- Verify documentation links work
- No code changes, so no tests needed

---

## Phase 2: Move Internal Packages (Medium Risk)

### Objective
Move application-specific packages to `internal/` directory.

### Steps

#### 2.1 Create Internal Package Directories

```bash
cd qwencoder-proxy/qwencoder-proxy
mkdir -p internal/provider internal/proxy internal/restapi internal/logging internal/converter internal/config internal/ratelimit
```

#### 2.2 Move Provider Package

```bash
# Move provider package
mv provider/* internal/provider/
rmdir provider
mv provider/antigravity internal/provider/
mv provider/gemini internal/provider/
mv provider/iflow internal/provider/
mv provider/kiro internal/provider/
mv provider/qwen internal/provider/
```

**Import Path Changes:**
```go
// Old
import "github.com/sunbankio/qwencoder-proxy/provider"

// New
import "github.com/sunbankio/qwencoder-proxy/internal/provider"
```

**Files to Update:**
- `cmd/main.go`
- `converter/converter.go`
- `restapi/rest_api.go`
- Any test files

#### 2.3 Move Proxy Package

```bash
# Move proxy package
mv proxy/* internal/proxy/
rmdir proxy
```

**Import Path Changes:**
```go
// Old
import "github.com/sunbankio/qwencoder-proxy/proxy"

// New
import "github.com/sunbankio/qwencoder-proxy/internal/proxy"
```

**Files to Update:**
- `cmd/main.go`
- `restapi/rest_api.go`
- Any test files

#### 2.4 Move RestAPI Package

```bash
# Move restapi package
mv restapi/* internal/restapi/
rmdir restapi
```

**Import Path Changes:**
```go
// Old
import "github.com/sunbankio/qwencoder-proxy/restapi"

// New
import "github.com/sunbankio/qwencoder-proxy/internal/restapi"
```

**Files to Update:**
- `cmd/main.go`
- Any test files

#### 2.5 Move Logging Package

```bash
# Move logging package
mv logging/* internal/logging/
rmdir logging
```

**Import Path Changes:**
```go
// Old
import "github.com/sunbankio/qwencoder-proxy/logging"

// New
import "github.com/sunbankio/qwencoder-proxy/internal/logging"
```

**Files to Update:**
- `cmd/main.go`
- `proxy/base.go` (after moving to internal)
- `restapi/rest_api.go` (after moving to internal)
- `internal/token/*.go`
- Any test files

#### 2.6 Move Converter Package

```bash
# Move converter package
mv converter/* internal/converter/
rmdir converter
```

**Import Path Changes:**
```go
// Old
import "github.com/sunbankio/qwencoder-proxy/converter"

// New
import "github.com/sunbankio/qwencoder-proxy/internal/converter"
```

**Files to Update:**
- `cmd/main.go`
- `restapi/rest_api.go`
- Any test files

#### 2.7 Move Config Package

```bash
# Move config package
mv config/* internal/config/
rmdir config
```

**Import Path Changes:**
```go
// Old
import "github.com/sunbankio/qwencoder-proxy/config"

// New
import "github.com/sunbankio/qwencoder-proxy/internal/config"
```

**Files to Update:**
- `cmd/main.go`
- `internal/token/*.go`
- Any test files

#### 2.8 Move RateLimit Package

```bash
# Move ratelimit package
mv ratelimit internal/ratelimit/
```

**Import Path Changes:**
```go
// Old
import "github.com/sunbankio/qwencoder-proxy/ratelimit"

// New
import "github.com/sunbankio/qwencoder-proxy/internal/ratelimit"
```

**Files to Update:**
- Any files that import ratelimit
- Any test files

#### 2.9 Update All Import Statements

Use a script to update all import statements:

```bash
# Create update script
cat > update_imports.sh << 'EOF'
#!/bin/bash

# Update provider imports
find . -name "*.go" -type f -exec sed -i 's|github.com/sunbankio/qwencoder-proxy/provider|github.com/sunbankio/qwencoder-proxy/internal/provider|g' {} +

# Update proxy imports
find . -name "*.go" -type f -exec sed -i 's|github.com/sunbankio/qwencoder-proxy/proxy|github.com/sunbankio/qwencoder-proxy/internal/proxy|g' {} +

# Update restapi imports
find . -name "*.go" -type f -exec sed -i 's|github.com/sunbankio/qwencoder-proxy/restapi|github.com/sunbankio/qwencoder-proxy/internal/restapi|g' {} +

# Update logging imports
find . -name "*.go" -type f -exec sed -i 's|github.com/sunbankio/qwencoder-proxy/logging|github.com/sunbankio/qwencoder-proxy/internal/logging|g' {} +

# Update converter imports
find . -name "*.go" -type f -exec sed -i 's|github.com/sunbankio/qwencoder-proxy/converter|github.com/sunbankio/qwencoder-proxy/internal/converter|g' {} +

# Update config imports
find . -name "*.go" -type f -exec sed -i 's|github.com/sunbankio/qwencoder-proxy/config|github.com/sunbankio/qwencoder-proxy/internal/config|g' {} +

# Update ratelimit imports
find . -name "*.go" -type f -exec sed -i 's|github.com/sunbankio/qwencoder-proxy/ratelimit|github.com/sunbankio/qwencoder-proxy/internal/ratelimit|g' {} +

EOF

# Make executable and run
chmod +x update_imports.sh
./update_imports.sh
```

#### 2.10 Verify Changes

```bash
# Check that all packages compile
go build ./...

# Run all tests
go test ./...

# Verify structure
ls -la internal/
```

### Testing After Phase 2

```bash
# Build the application
go build -o qwencoder-proxy ./cmd/qwencoder-proxy

# Run all tests
go test ./... -v

# Run with race detector
go test ./... -race
```

---

## Phase 3: Flatten Directory Structure (High Risk)

### Objective
Move everything from `qwencoder-proxy/qwencoder-proxy/` to `qwencoder-proxy/`.

### Steps

#### 3.1 Backup Current State

```bash
# Create a tarball backup
cd qwencoder-proxy
tar -czf ../qwencoder-proxy-backup-$(date +%Y%m%d).tar.gz qwencoder-proxy/
```

#### 3.2 Move Files to Root

```bash
# Move all contents of qwencoder-proxy/qwencoder-proxy/ to qwencoder-proxy/
cd qwencoder-proxy
mv qwencoder-proxy/* .
mv qwencoder-proxy/.* . 2>/dev/null || true

# Remove empty directory
rmdir qwencoder-proxy
```

#### 3.3 Update go.mod

The `go.mod` file should already be correct since we're moving it to the workspace root. Verify:

```bash
# Check go.mod
cat go.mod | head -5

# Should show:
# module github.com/sunbankio/qwencoder-proxy
```

#### 3.4 Verify Import Paths

Since we're flattening the structure but keeping the same module path, import paths should remain the same:

```go
import "github.com/sunbankio/qwencoder-proxy/internal/provider"
import "github.com/sunbankio/qwencoder-proxy/internal/proxy"
// etc.
```

#### 3.5 Update Build Scripts

Update any build scripts that reference the nested structure:

- `Makefile`
- CI/CD configuration files
- Dockerfiles
- Any deployment scripts

#### 3.6 Verify Changes

```bash
# Check structure
ls -la

# Should show:
# cmd/
# internal/
# web/
# docs/
# go.mod
# go.sum
# Makefile
# etc.
```

### Testing After Phase 3

```bash
# Clean build
go clean -cache
go mod tidy
go build ./...

# Run all tests
go test ./... -v

# Run with race detector
go test ./... -race

# Build the application
go build -o qwencoder-proxy ./cmd/qwencoder-proxy
```

---

## Phase 4: Cleanup and Finalization (Low Risk)

### Objective
Clean up any remaining issues and finalize the restructuring.

### Steps

#### 4.1 Remove Empty Directories

```bash
# Find and remove empty directories
find . -type d -empty -delete
```

#### 4.2 Update Documentation

Update documentation to reflect the new structure:

- `README.md`
- `docs/api-reference.md`
- Any architecture documentation
- Update import path examples in documentation

#### 4.3 Update CI/CD Pipelines

Update CI/CD configuration files:
- `.github/workflows/*.yml`
- Any other CI configuration

#### 4.4 Update IDE Configuration

If using IDE-specific configuration:
- `.vscode/settings.json`
- `.idea/` (if using IntelliJ/GoLand)
- Any other IDE configuration

#### 4.5 Final Testing

```bash
# Clean build
go clean -cache
go mod tidy

# Build all packages
go build ./...

# Run all tests with coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html

# Run benchmarks
go test ./... -bench=. -benchmem

# Verify the application runs
./qwencoder-proxy --help
```

#### 4.6 Commit Changes

```bash
# Stage all changes
git add .

# Commit with descriptive message
git commit -m "refactor: restructure project to follow Go best practices

- Move all internal packages to internal/ directory
- Flatten directory structure (remove nested qwencoder-proxy/)
- Reorganize documentation (plans, todo, bugs under docs/)
- Update all import paths
- Update build scripts and documentation

This change improves code organization and follows Go's standard
project layout, making the codebase more maintainable and easier
to understand.

Breaking changes:
- All internal package imports now use internal/ prefix
- Project structure flattened, go.mod now at workspace root"
```

---

## Rollback Plan

If issues arise during migration, you can rollback:

### Rollback to Pre-Migration State

```bash
# Restore from backup
cd ..
tar -xzf qwencoder-proxy-backup-YYYYMMDD.tar.gz

# Remove the failed migration
rm -rf qwencoder-proxy

# Restore from backup
mv qwencoder-proxy-backup-YYYYMMDD qwencoder-proxy

# Checkout original branch
cd qwencoder-proxy
git checkout main
```

### Rollback After Each Phase

If you want to rollback after a specific phase:

1. **After Phase 1:** Just move documentation back
2. **After Phase 2:** Move packages back from `internal/` to root
3. **After Phase 3:** Restore from backup (most reliable)

---

## Post-Migration Checklist

- [ ] All tests pass: `go test ./...`
- [ ] Application builds successfully: `go build ./...`
- [ ] No compilation errors
- [ ] Documentation updated
- [ ] CI/CD pipelines updated and passing
- [ ] Team members notified of changes
- [ ] Import paths verified in all files
- [ ] No empty directories remaining
- [ ] Git history preserved (if using git mv)

---

## Estimated Timeline

| Phase | Estimated Time | Risk Level |
|-------|---------------|------------|
| Phase 1 | 30 minutes | Low |
| Phase 2 | 2-3 hours | Medium |
| Phase 3 | 1-2 hours | High |
| Phase 4 | 1-2 hours | Low |
| **Total** | **5-8 hours** | - |

---

## Notes and Considerations

1. **Git History:** Using `git mv` instead of regular `mv` commands will preserve file history. Consider using `git mv` for all file moves.

2. **Testing:** Run tests after each package move to catch issues early.

3. **Team Coordination:** Coordinate with team members to avoid conflicts during the migration.

4. **Branching:** Consider doing this in a feature branch and creating a pull request for review.

5. **Documentation:** Update all documentation that references the old structure.

6. **External Dependencies:** Check if any external tools or scripts depend on the directory structure.

7. **IDE Settings:** IDEs may need to be refreshed after the restructuring.

---

## Success Criteria

The migration is successful when:

1. All code compiles without errors
2. All tests pass
3. The application runs correctly
4. Import paths are consistent
5. Documentation is updated
6. CI/CD pipelines pass
7. No empty directories remain
8. The structure follows Go best practices

---

## Contact

For questions or issues during the migration, refer to:
- Go Project Layout: https://github.com/golang-standards/project-layout
- Go Internal Package Convention: https://go.dev/doc/go1.4#internalpackages
- Project Structure Analysis: `docs/architecture/project-structure-analysis.md`
