# Builds the centos-streamed user-facing web server (./cmd/web).
#
# The build context is the repository root. The platform CLI generates a
# Quadlet .build unit (hello.build) that runs `podman build` with this file,
# then hello.container runs the resulting image on the shared proxy network.

FROM docker.io/library/golang:1.25 AS build
WORKDIR /src

# Copy the module and build the web binary. The app is stdlib-only, so no
# module downloads happen; the build is fully offline after the base image.
COPY go.mod ./
COPY cmd/ ./cmd/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /web ./cmd/web

# Minimal runtime image. alpine keeps a shell for debugging; the static binary
# needs no libc, and /proc is provided by the container runtime at runtime.
FROM docker.io/library/alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /web /usr/local/bin/web

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/web"]
