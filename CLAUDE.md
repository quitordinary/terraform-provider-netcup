# terraform-provider-netcup

The OpenTofu provider for netcup's SCP and the `netcupctl` CLI, one Go module.
Why and how it is used: [ADR-0066](https://github.com/quitordinary/Infrastructure/blob/main/decisions/0066-own-netcup-provider.md).
The platform's rules live in `quitordinary/Infrastructure` (`PLATFORM.md`); the work is
issues #67–#74 there, not in this repository.

This repository is **public**. Everything in it, history included, is world-readable.

## Non-negotiables

- **No token is ever printed, logged or written to a file** by the code, the tests or
  you. `netcupctl login` writes the refresh token to stdout only, never to a terminal.
  Errors carry status and a bounded body excerpt, never headers. Tests assert this.
- **Nothing here calls the live netcup API** except an operator running `netcupctl` on
  the workstation. CI and `make check` use the fake in `internal/scptest`.
- **Dependencies:** `golang.org/x/oauth2` and, with the provider, the HashiCorp plugin
  packages. Nothing else without an ADR-0066 amendment. Tools go in `tools/go.mod`,
  never in the main module. Every new version is at least 7 days old.
- **Actions:** GitHub-owned only (the organisation allowlist has no others for this
  repository), pinned to commit SHAs, `permissions` least privilege,
  `persist-credentials: false`, no `pull_request_target`. Tools run as `go tool`.
- **Declarative state goes in the provider; operations go in `netcupctl`.** No resource
  that only performs an action. No provider actions until OpenTofu has them
  (opentofu#3309).
- **Imperative operations live in one Go function each**, shared by `netcupctl`.

## Working here

```bash
make check    # tidy-diff, vet, race tests, golangci-lint, govulncheck
make build    # bin/netcupctl
```

- Commit straight to `main` (one operator, ADR-0056); `main` cannot be force-pushed or
  deleted. Release tags `v*` are created only by organisation admins.
- The SCP returns `null` for some fields the spec marks required (`rdns`,
  `interfaces[].vlanId`): pointer types and a test each.
- Fakes write the real endpoint paths out instead of importing them, so a test pins
  the contract.
