# centos-streamed

A tiny Cockpit-style **system monitor**: a single Go binary that serves a live
web view of the machine it runs on — host facts and the running process list —
pushed to the browser over server-sent events.

No database, no reverse proxy, no deploy tooling. Just a webserver reading
`/proc`.

## Structure

```
centos-streamed/
├── env.go                 # config (PORT), package streamed
├── cmd/main.go            # entrypoint → web.RunBlocking
├── web/
│   ├── httpServer.go      # chi routes + graceful HTTP server
│   ├── httpGet.go         # home page + SSE loop (2s refresh)
│   ├── serverinfo.go      # host facts from os-release / /proc / runtime
│   ├── procinfo.go        # process list from /proc/<pid>
│   ├── *.templ            # layout, home (info card + process table), 404
│   └── static/            # CSS + theme JS (embedded, content-hashed)
└── go.mod
```

## How it works

`cmd/main.go` starts a [chi] HTTP server. The home page renders a host-info card
and a process table; a `data-init` [Datastar] attribute opens an SSE connection
to `/sse`, which every two seconds re-reads `/proc` and patches both fragments
back into the page live.

- **Host facts** — hostname, `PRETTY_NAME` from `/etc/os-release`, kernel from
  `/proc/sys/kernel/osrelease`, plus memory/uptime from `/proc` and CPUs/arch
  from the Go runtime.
- **Processes** — walks `/proc/<pid>/{status,stat}` for each process: command,
  resolved user, state, threads, resident memory and cumulative CPU time,
  sorted by memory (top 40).

These reads only return data on **Linux**, so run it inside the VM, not on macOS.

[chi]: https://github.com/go-chi/chi
[Datastar]: https://data-star.dev
[templ]: https://templ.guide

## Run

Dev loop with hot reload (regenerates templ + rebuilds on save):

```bash
task setup   # once: install pinned templ / air tools into go.mod
task         # runs air
```

Or build and run directly:

```bash
task go:build        # -> ./release/web
go run ./cmd
```

Then open **http://localhost:44223**. Config is `STREAMED_`-prefixed (see
`env.go`); override via `.env`.

## Development on macOS (Lima VM)

`/proc` only exists on Linux, so run inside the `centos10` VM. The repo is
mounted at `/home/lima/centos10/centos-streamed` and guest port 44223 is
forwarded to the host:

```bash
limactl shell centos10 sh -c 'cd /home/lima/centos10/centos-streamed && go run ./cmd'
```

Then open **http://localhost:44223** on the Mac.
