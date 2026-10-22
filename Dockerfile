# Stage 1: compile. This image has the whole Go toolchain (~800 MB), but none
# of it ends up in the final image.
FROM golang:1.27 AS build
WORKDIR /src

# Copy go.mod first so the dependency layer is cached between code changes.
COPY go.mod ./
RUN go mod download

COPY . .
# CGO_ENABLED=0 gives a fully static binary that needs no libc at runtime.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/raftkv ./cmd/raftkv
# distroless has no shell to mkdir with, so make the data dir here.
RUN mkdir /out/data

# Stage 2: run. Just the binary on a minimal base with no shell or package
# manager, running as a non-root user (uid 65532).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/raftkv /raftkv
COPY --from=build --chown=65532:65532 /out/data /data

# The WAL lives here. Anything not on a volume is lost when the container goes.
VOLUME /data
EXPOSE 8080

# Listen on all interfaces: "localhost" inside a container is unreachable
# from outside it.
ENTRYPOINT ["/raftkv", "-addr", ":8080", "-data-dir", "/data"]
