# centos-streamed

A tiny Go platform that runs containers directly on **systemd + Podman (Quadlet)**,
fronted by **Caddy** for HTTPS. One CLI generates all the host config and applies it.

## Structure

```
centos-streamed/
├── Containerfile              # builds ./cmd/web (templ generate + go build)
├── env.go                     # web-server config (package streamed)
├── sqlc.yaml  Taskfile.yml  .air.toml
├── cmd/
│   ├── platform/              # the reconciler CLI (generates host wiring)
│   │   ├── main.go
│   │   └── templates/         # embedded Quadlet + Caddy files (go:embed)
│   │       ├── proxy.network
│   │       ├── caddy-data.volume
│   │       ├── caddy-config.volume
│   │       ├── caddy.container
│   │       ├── Caddyfile
│   │       ├── streamed.build     # {{.Image}}, {{.BuildContext}}
│   │       ├── streamed.container # {{env "SERVER_NAME" .ServerName}} …
│   │       └── streamed.caddy     # {{.Host}}, {{- if .TLSInternal}}
│   └── web/main.go            # thin entrypoint → web.RunBlocking
├── web/                       # the Datastar app (chi routes, templ views, SSE)
│   ├── httpServer.go  httpGet.go  httpPost.go  nats.go  serverinfo.go
│   ├── *.templ               # layout, home, 404 (templ → *_templ.go)
│   └── static/               # open-props CSS + theme JS (embedded, hashfs)
├── sql/                       # goose migrations + sqlc-generated queries
│   ├── db.go  migrations/  queries/  sqlcgen/
└── go.mod
```

The CLI renders `templates/` → `/etc/containers/systemd/*` and `/etc/caddy/*`,
then reloads systemd, starts the services, and reloads Caddy.

## The web app

`./cmd/web` is a **Datastar-driven server** on the same stack as the `basicauth`
reference project — [chi] routing, [goose] migrations, [sqlc] queries, [templ]
views, an embedded [NATS] pub/sub bus, and [hashfs] content-hashed static assets
— **without any authentication**. It serves:

- a **live server-info card** (host name/OS/kernel/memory/uptime), pushed over
  SSE and refreshed every second, and
- a **live login/activity feed** — each time someone loads the app it records a
  `visit` event (client IP + timestamp) to the SQLite `events` table and publishes
  on NATS, which fans out to every open SSE connection so all clients see the new
  login appear live.

Host facts (`SERVER_NAME`, `SERVER_OS`, `SERVER_KERNEL`) are injected by the
platform CLI onto `streamed.container`; the `/proc`-based fields (memory, uptime)
reflect the host and only populate inside the Linux container.

[chi]: https://github.com/go-chi/chi
[goose]: https://github.com/pressly/goose
[sqlc]: https://sqlc.dev
[templ]: https://templ.guide
[NATS]: https://nats.io
[hashfs]: https://github.com/benbjohnson/hashfs

## Commands

```bash
# Deploy: render files, build image, start services, reload Caddy
sudo go run ./cmd/platform

# Options
sudo go run ./cmd/platform -host app.example.com -tls auto   # real domain + Let's Encrypt
sudo go run ./cmd/platform -generate-only                    # write files only, no apply

# Clean slate (stop + remove units, containers, image, volumes, network)
sudo go run ./cmd/platform -teardown
```

## Web dev (fast loop, no containers)

Iterate on the web app directly with hot reload — no Podman/Caddy needed:

```bash
task setup   # once: install pinned templ / sqlc / air tools into go.mod
task         # sqlc generate, then `air` (regenerates templ + rebuilds on save)
```

Then open **http://localhost:8080**. Config (all `STREAMED_`-prefixed) lives in
`env.go`; override via a `.env` file. Other tasks: `task sqlc`, `task templ:build`,
`task go:build` (production binary → `./release/web`), `task clean`.

## For Development (Mac OS)
Run from the repo root inside the VM (`limactl shell centos10`):

Test loop: after editing `cmd/web`, just re-run `sudo go run ./cmd/platform`
(`-teardown` clears the image so changes always rebuild).

Podman publishes ports via nft DNAT, which Lima can't auto-forward, so tunnel:

```bash
ssh -F ~/.lima/centos10/ssh.config -N -L 8443:127.0.0.1:443 lima-centos10
```

Then open **https://localhost:8443** (accept the `tls internal` cert warning).
