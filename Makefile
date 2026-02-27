.PHONY: test
test:
go test -v ./...

.PHONY: test-sqlite
test-sqlite:
go test -v ./internal/token/ -run "TestSQLiteStore|TestMigrator"

.PHONY: test-integration
test-integration:
go test -v ./internal/token/ -run "TestIntegration"

.PHONY: test-coverage
test-coverage:
./scripts/test_coverage.bat

.PHONY: benchmark
benchmark:
go test -bench=. -benchmem ./internal/token/ -run "Benchmark"

.PHONY: test-race
test-race:
go test -race ./internal/token/
