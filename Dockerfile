# punchflow container image (local and development use only).
# Build: docker buildx build --platform linux/amd64,linux/arm64 -t tacogips/punchflow:latest .

FROM --platform=$BUILDPLATFORM oven/bun:1.4.3 AS web
WORKDIR /src/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

FROM --platform=$BUILDPLATFORM golang:1.25.4-alpine AS server
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w" -o /out/punchflow ./cmd/punchflow \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/punchflow /usr/local/bin/punchflow
COPY --from=server --chown=nonroot:nonroot /out/data /data
ENV PUNCHFLOW_LISTEN=0.0.0.0:8080 \
    PUNCHFLOW_DATA_DIR=/data
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/punchflow"]
CMD ["serve"]
