FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/project-scientist ./cmd/project-scientist

FROM alpine:3.22
RUN addgroup -S scientist && adduser -S scientist -G scientist && mkdir -p /data /app/web && chown -R scientist:scientist /data /app
WORKDIR /app
COPY --from=build /out/project-scientist /app/project-scientist
COPY --chown=scientist:scientist web ./web
USER scientist
ENV PSC_ADDR=:8080 PSC_DATA_DIR=/data
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
CMD ["/app/project-scientist"]
