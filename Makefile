.PHONY: check-gmail-oauth-client build-gmail install-gmail

gmail-ldflags = -X github.com/intelligrit/mail-app-cli/internal/gmailauth.appOAuthClientID=$(GMAIL_OAUTH_CLIENT_ID) -X github.com/intelligrit/mail-app-cli/internal/gmailauth.appOAuthClientSecret=$(GMAIL_OAUTH_CLIENT_SECRET)

check-gmail-oauth-client:
	@test -n "$(strip $(GMAIL_OAUTH_CLIENT_ID))" || (echo "GMAIL_OAUTH_CLIENT_ID is required" >&2; exit 1)
	@test -n "$(strip $(GMAIL_OAUTH_CLIENT_SECRET))" || (echo "GMAIL_OAUTH_CLIENT_SECRET is required" >&2; exit 1)

build: check-gmail-oauth-client
	@go build -ldflags "$(gmail-ldflags)"

install: check-gmail-oauth-client
	@go install -ldflags "$(gmail-ldflags)"

.PHONY: lint

GOLANGCI_LINT ?= golangci-lint
GOLANGCI_LINT_VERSION := 2.15.0

lint:
	@$(GOLANGCI_LINT) version | grep -Eq ' version v?$(subst .,[.],$(GOLANGCI_LINT_VERSION)) ' || { \
		echo "Install golangci-lint v$(GOLANGCI_LINT_VERSION) or set GOLANGCI_LINT to its path." >&2; \
		exit 1; \
	}
	@$(GOLANGCI_LINT) run ./...
