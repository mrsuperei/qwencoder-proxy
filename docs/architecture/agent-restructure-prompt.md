# Agent Restructuring Prompt

Copy and paste the following prompt to your agent to restructure the qwencoder-proxy project:

---

## Restructure qwencoder-proxy Project to Follow Go Best Practices

You are tasked with restructuring the qwencoder-proxy project to follow Go best practices. The current structure has several issues:

1. **Nested directory structure**: The Go module is at `qwencoder-proxy/qwencoder-proxy/` instead of the workspace root
2. **Internal packages at top level**: Packages like `provider/`, `proxy/`, `restapi/`, `logging/`, `converter/`, `config/`, and `ratelimit/` should be in `internal/` but are publicly importable
3. **Mixed concerns at root**: Documentation, plans, bugs, and application code are all mixed at the top level

## Target Structure

After restructuring, the project should have this structure:

```
qwencoder-proxy/                    # Go module root (go.mod at workspace root)
├── cmd/
│   └── qwencoder-proxy/
│       └── main.go                # Application entry point
├── internal/                       # All internal packages (not importable externally)
│   ├── config/                    # Configuration management
│   ├── provider/                  # Provider implementations
│   │   ├── provider.go
│   │   ├── factory.go
│   │   ├── base.go
│   │   ├── antigravity/
│   │   ├── gemini/
│   │   ├── iflow/
│   │   ├── kiro/
│   │   └── qwen/
│   ├── proxy/                     # HTTP proxy handlers
│   │   ├── base.go
│   │   ├── common.go
│   │   ├── openai_handler.go
│   │   ├── gemini_handler.go
│   │   ├── anthropic_handler.go
│   │   └── stream_converter.go
│   ├── restapi/                   # Admin REST API
│   │   ├── rest_api.go
│   │   ├── middleware.go
│   │   ├── provider_registry.go
│   │   └── state_manager.go
│   ├── logging/                   # Logging infrastructure
│   │   ├── logger.go
│   │   ├── interface.go
│   │   └── debug.go
│   ├── converter/                 # Format conversion
│   │   ├── converter.go
│   │   ├── openai.go
│   │   ├── gemini.go
│   │   ├── claude.go
│   │   ├── qwen.go
│   │   └── helpers.go
│   ├── ratelimit/                 # Rate limiting
│   │   ├── api/
│   │   ├── provider/
│   │   └── store/
│   └── token/                     # Token management (already in internal/)
├── web/                           # Static web assets
│   └── dashboard/
├── docs/                          # Documentation
│   ├── architecture/
│   ├── plans/                     # Move from root
│   ├── todo/                      # Move from root
│   └── bugs/                      # Move from root
├── scripts/                       # Build and utility scripts
├── .github/                       # GitHub-specific files
├── .credentials/                  # Credentials (gitignored)
├── go.mod                         # Move to workspace root
├── go.sum                         # Move to workspace root
├── Makefile                       # Move to workspace root
├── .gitignore                     # Move to workspace root
└── README.md                      # Move to workspace root
```

## Instructions

### Phase 1: Documentation Reorganization (Low Risk)

1. Create documentation subdirectories:
   - `qwencoder-proxy/docs/plans/`
   - `qwencoder-proxy/docs/todo/`
   - `qwencoder-proxy/docs/bugs/`
   - `qwencoder-proxy/docs/architecture/`

2. Move project management files:
   - Move all files from `qwencoder-proxy/plans/` to `qwencoder-proxy/docs/plans/`
   - Move all files from `qwencoder-proxy/todo/` to `qwencoder-proxy/docs/todo/`
   - Move all files from `qwencoder-proxy/bugs/` to `qwencoder-proxy/docs/bugs/`

3. Remove empty directories: `plans/`, `todo/`, `bugs/`

### Phase 2: Move Internal Packages (Medium Risk)

For each of the following packages, perform these steps:

1. **Move provider package:**
   - Move `qwencoder-proxy/provider/*` to `qwencoder-proxy/internal/provider/`
   - Move `qwencoder-proxy/provider/antigravity/` to `qwencoder-proxy/internal/provider/`
   - Move `qwencoder-proxy/provider/gemini/` to `qwencoder-proxy/internal/provider/`
   - Move `qwencoder-proxy/provider/iflow/` to `qwencoder-proxy/internal/provider/`
   - Move `qwencoder-proxy/provider/kiro/` to `qwencoder-proxy/internal/provider/`
   - Move `qwencoder-proxy/provider/qwen/` to `qwencoder-proxy/internal/provider/`
   - Update all import statements from `github.com/sunbankio/qwencoder-proxy/provider` to `github.com/sunbankio/qwencoder-proxy/internal/provider`

2. **Move proxy package:**
   - Move `qwencoder-proxy/proxy/*` to `qwencoder-proxy/internal/proxy/`
   - Update all import statements from `github.com/sunbankio/qwencoder-proxy/proxy` to `github.com/sunbankio/qwencoder-proxy/internal/proxy`

3. **Move restapi package:**
   - Move `qwencoder-proxy/restapi/*` to `qwencoder-proxy/internal/restapi/`
   - Update all import statements from `github.com/sunbankio/qwencoder-proxy/restapi` to `github.com/sunbankio/qwencoder-proxy/internal/restapi`

4. **Move logging package:**
   - Move `qwencoder-proxy/logging/*` to `qwencoder-proxy/internal/logging/`
   - Update all import statements from `github.com/sunbankio/qwencoder-proxy/logging` to `github.com/sunbankio/qwencoder-proxy/internal/logging`

5. **Move converter package:**
   - Move `qwencoder-proxy/converter/*` to `qwencoder-proxy/internal/converter/`
   - Update all import statements from `github.com/sunbankio/qwencoder-proxy/converter` to `github.com/sunbankio/qwencoder-proxy/internal/converter`

6. **Move config package:**
   - Move `qwencoder-proxy/config/*` to `qwencoder-proxy/internal/config/`
   - Update all import statements from `github.com/sunbankio/qwencoder-proxy/config` to `github.com/sunbankio/qwencoder-proxy/internal/config`

7. **Move ratelimit package:**
   - Move `qwencoder-proxy/ratelimit` to `qwencoder-proxy/internal/ratelimit/`
   - Update all import statements from `github.com/sunbankio/qwencoder-proxy/ratelimit` to `github.com/sunbankio/qwencoder-proxy/internal/ratelimit`

**Important:** After moving each package, verify:
- `go build ./...` succeeds
- `go test ./...` passes

### Phase 3: Flatten Directory Structure (High Risk)

1. **Create a backup** before proceeding:
   - Create a tarball backup of the current state

2. **Move all files to workspace root:**
   - Move all contents from `qwencoder-proxy/qwencoder-proxy/` to `qwencoder-proxy/`
   - This includes: `cmd/`, `internal/`, `web/`, `docs/`, `scripts/`, `.github/`, `.credentials/`, `go.mod`, `go.sum`, `Makefile`, `.gitignore`, `README.md`, and any other files

3. **Remove the empty nested directory:**
   - Remove `qwencoder-proxy/qwencoder-proxy/`

4. **Verify go.mod is correct:**
   - Ensure `go.mod` is at the workspace root
   - Verify the module path is `github.com/sunbankio/qwencoder-proxy`

### Phase 4: Cleanup and Finalization (Low Risk)

1. **Remove empty directories:**
   - Find and remove any empty directories

2. **Update documentation:**
   - Update `README.md` to reflect the new structure
   - Update any documentation that references the old structure

3. **Update build scripts:**
   - Update `Makefile` to reflect the new structure
   - Update any CI/CD configuration files

4. **Final verification:**
   - Run `go clean -cache`
   - Run `go mod tidy`
   - Run `go build ./...`
   - Run `go test ./... -v`
   - Run `go test ./... -race`
   - Build the application: `go build -o qwencoder-proxy ./cmd/qwencoder-proxy`

## Import Path Changes Summary

| Old Import Path | New Import Path |
|-----------------|-----------------|
| `github.com/sunbankio/qwencoder-proxy/provider` | `github.com/sunbankio/qwencoder-proxy/internal/provider` |
| `github.com/sunbankio/qwencoder-proxy/proxy` | `github.com/sunbankio/qwencoder-proxy/internal/proxy` |
| `github.com/sunbankio/qwencoder-proxy/restapi` | `github.com/sunbankio/qwencoder-proxy/internal/restapi` |
| `github.com/sunbankio/qwencoder-proxy/logging` | `github.com/sunbankio/qwencoder-proxy/internal/logging` |
| `github.com/sunbankio/qwencoder-proxy/converter` | `github.com/sunbankio/qwencoder-proxy/internal/converter` |
| `github.com/sunbankio/qwencoder-proxy/config` | `github.com/sunbankio/qwencoder-proxy/internal/config` |
| `github.com/sunbankio/qwencoder-proxy/ratelimit` | `github.com/sunbankio/qwencoder-proxy/internal/ratelimit` |

## Files That Need Import Path Updates

The following files will need their import statements updated:

- `qwencoder-proxy/cmd/main.go`
- `qwencoder-proxy/internal/converter/converter.go`
- `qwencoder-proxy/internal/restapi/rest_api.go`
- `qwencoder-proxy/internal/proxy/base.go`
- `qwencoder-proxy/internal/token/*.go` (multiple files)
- All test files in the affected packages

## Success Criteria

The restructuring is complete when:

1. All code compiles without errors: `go build ./...`
2. All tests pass: `go test ./...`
3. The application builds successfully
4. Import paths are consistent throughout the codebase
5. Documentation is updated
6. No empty directories remain
7. The structure follows Go best practices with `internal/` used correctly

## Notes

- Use `git mv` instead of regular `mv` commands to preserve git history
- Test after each package move to catch issues early
- The module path `github.com/sunbankio/qwencoder-proxy` remains unchanged
- Only the physical file structure changes, not the import paths (except for adding `internal/` prefix)

## Rollback

If issues arise, you can rollback by:
1. Restoring from the backup created before Phase 3
2. Or reversing the moves in reverse order (Phase 4 → Phase 3 → Phase 2 → Phase 1)

Begin with Phase 1 and proceed sequentially through each phase, testing thoroughly after each phase.
