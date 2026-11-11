# terraform-provider-netcup

An OpenTofu provider for the state of netcup servers that stays put (hostname, vLAN
attachment, rDNS, SSH keys), and `netcupctl` for what is not state (the login, ISO
attach, rescue, power). Both talk to netcup's [SCP REST
API](https://www.servercontrolpanel.de/scp-core/api/v1). Built for quitordinary's own
platform; why it exists and how it is installed is
[ADR-0066](https://github.com/quitordinary/Infrastructure/blob/main/decisions/0066-own-netcup-provider.md).

**Status:** before the spike
([Infrastructure#68](https://github.com/quitordinary/Infrastructure/issues/68)).
Only `netcupctl login`, `token check`, `token renew` and `spec` exist; there is no
provider yet.

## netcupctl

```bash
make build                                         # bin/netcupctl
export NETCUP_SCP_REFRESH_TOKEN="$(bin/netcupctl login)"
bin/netcupctl token check                          # lifetimes and expiry, never the token
bin/netcupctl token check --reuse                  # also: is the old token still accepted?
bin/netcupctl token renew | <store>                # the newest token, for the store (never to a terminal)
bin/netcupctl spec fetch -o openapi.json
bin/netcupctl spec paths openapi.json              # METHOD path, one per operation
```

`login` prints the browser link and code to stderr and the refresh token only to
stdout, and refuses when stdout is a terminal; so does `renew`. A refresh token expires
30 days after its own issue and every exchange returns a newer one, so whatever stores
the token stores the newest. `--reuse` presents a refresh token twice;
if the realm detects reuse it revokes the session, and you log in again.

## Development

```bash
make check    # tidy, vet, race tests, golangci-lint, govulncheck; no credentials, no network to netcup
```

Tools are pinned in [`tools/go.mod`](tools/go.mod), apart from the provider's own
dependencies.

## Licence

[Apache-2.0](LICENSE). See [NOTICE](NOTICE).
