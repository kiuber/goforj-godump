.PHONY: modernize modernize-fix modernize-check linter-run

MODERNIZE_CMD = go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@v0.18.1

modernize: modernize-fix

modernize-fix:
	@echo "Running gopls modernize with -fix..."
	$(MODERNIZE_CMD) -test -fix ./...

modernize-check:
	@echo "Checking if code needs modernization..."
	$(MODERNIZE_CMD) -test ./...

linter-run:
	@echo "Running linter..."
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1 run -v
	@echo "Linter run complete."