# MailCLI development

MailCLI is a Go CLI for macOS Mail.app. Cobra commands live in `cmd/`.
The Mail.app client and embedded AppleScript and JXA scripts live in
`pkg/mail/`. Gmail authentication and archiving live in `internal/`.

## Build and verify

- Use macOS with Go and the Command Line Tools. Build locally with `go build .`.
- Install the golangci-lint version pinned in `Makefile`. Run `make lint`
  and `go test ./...` on macOS after each feature change. Format changed Go
  files with `gofmt`.
- For changes to the scripting bridge, run the affected commands against a
  test mailbox in Mail.app. Automated script compilation checks syntax.
- Use `make build` for Gmail-enabled builds with the maintainer's OAuth build
  configuration. Keep credentials out of source, command output, and fixtures.
