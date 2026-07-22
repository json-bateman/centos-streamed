# Builds the centos-streamed user-facing web server (./cmd/web).
#
# The build context is the repository root. The platform CLI generates a
# Quadlet .build unit (streamed.build) that runs `podman build` with this file,
# then streamed.container runs the resulting image on the shared proxy network.
#
# The app is a Datastar/chi/goose/sqlc/templ server. sqlc output is committed,
# but templ output (*_templ.go) is generated here at build time. The SQLite
# driver is pure Go (modernc.org/sqlite), so the binary stays static (CGO off).

FROM docker.io/library/golang:1.26 AS build
WORKDIR /src

# Download modules first so they cache across source-only changes.
COPY go.mod go.sum ./
RUN go mod download

# Pin templ to the version declared as a tool in go.mod, then build.
RUN go install github.com/a-h/templ/cmd/templ@v0.3.1020

COPY . .
RUN templ generate
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /web ./cmd/web

# Minimal runtime image. alpine keeps a shell for debugging; the static binary
# needs no libc, and /proc is provided by the container runtime at runtime.
FROM docker.io/library/alpine:3.20
RUN adduser -D -u 10001 app
WORKDIR /app
# The server creates ./data/streamed.db on start, so the workdir must be writable.
RUN chown app:app /app
USER app
COPY --from=build /web /usr/local/bin/web

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/web"]
