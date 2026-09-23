# north

Your [LogNorth](https://lognorth.com) server, in your terminal. `north tail` follows the production log. `north top` is htop for your app: its endpoints, the alerts firing now, and the uptime ping.

```bash
curl -fsSL https://lognorth.com/cli | sh
```

It picks the build for your Mac or Linux machine, checks it against the release checksum, installs `north` without sudo, and asks for your LogNorth URL and an agent key from **Settings > Developer**.

```bash
north tail --errors            # failures, live
north tail --path /checkout    # one endpoint
north top                      # j/k move, enter shows errors, w window, q quit
```

`north` reads the same read-only MCP tools your coding agent reads, with the same agent key, so it can look but never touch. It needs LogNorth v0.20.0 or later on the server.

Full docs: [lognorth.com/docs/features/terminal](https://lognorth.com/docs/features/terminal/)

## Build

```bash
go test ./...
go build -o north .
```

Tagging `v*` releases `north-<os>-<arch>` binaries and `checksums.txt` through GitHub Actions.

## License

MIT
