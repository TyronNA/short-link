# Build a static, CGO-free binary (the SQLite driver is pure Go).
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/shortlink ./cmd/server
# Pre-create the data dir owned by the runtime user (uid 65532 == distroless
# "nonroot"). distroless has no shell, so we cannot mkdir/chown there; staging
# it here and COPY --chown below is the only way to give nonroot a writable
# /data when it is mounted as an (anonymous) volume.
RUN mkdir -p /out/data && chown 65532:65532 /out/data

# Minimal runtime image.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/shortlink /shortlink
COPY --from=build --chown=65532:65532 /out/data /data
ENV PORT=8080 DB_PATH=/data/shortlink.db
EXPOSE 8080
# /data persists the SQLite file across restarts; mount a volume here. It is
# owned by nonroot so the process can create/write the DB file.
VOLUME ["/data"]
USER nonroot:nonroot
ENTRYPOINT ["/shortlink"]
