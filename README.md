# north

[![CI](https://github.com/karloscodes/lognorth-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/karloscodes/lognorth-cli/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/karloscodes/lognorth-cli)](https://github.com/karloscodes/lognorth-cli/releases/latest) [![License](https://img.shields.io/github/license/karloscodes/lognorth-cli)](LICENSE)

The [LogNorth](https://lognorth.com) CLI: your LogNorth server, in your terminal. `north tail` follows the production log. `north top` is htop for your app: its endpoints, the alerts firing now, and the uptime ping.

## Install

```bash
curl -fsSL https://lognorth.com/cli | sh
```

The installer picks the build for your Mac or Linux machine and checks it against the release checksum. It installs `north` without sudo. Then it asks for your LogNorth URL and an agent key from **Settings > Developer**.

## Use

```bash
north tail --errors            # failures, live
north tail --path /checkout    # one endpoint
north tail --app name          # one app, by name or id
north top                      # j/k move, enter shows errors, w changes the window, q quits
north top --app name           # start on one app
```

## Coding agents

`north connect` adds LogNorth to the coding agents it finds: Claude Code, Codex, Gemini CLI, and Cursor. Each agent starts `north mcp`, which relays its calls to your server. The URL and key stay in `~/.config/lognorth/remote.json`, readable only by you. They never go into an agent config.

```bash
north agents                   # add LogNorth to the agents on this machine
north call list_alerts         # run one tool and print its JSON
north update                   # update north to the latest release
```

`north` uses the same read-only MCP tools as your coding agent, with the same agent key. It reads data and never changes it. It needs LogNorth v0.20.0 or later on the server.

Full docs: [lognorth.com/docs/features/terminal](https://lognorth.com/docs/features/terminal/)

## Development

```bash
go test ./...
go build -o north .
```

## Releases

A tag `v*` starts a release. GitHub Actions builds `north-<os>-<arch>` binaries and `checksums.txt`.

## License

MIT. See [LICENSE](LICENSE).
