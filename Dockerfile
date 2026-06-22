# Project Scientist local-only development/runtime image.
# Base images are pinned by digest for deterministic rebuilds; update deliberately.
FROM golang:1.26-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648 AS build
ARG GO_PARALLEL=2
ENV GOMAXPROCS=${GO_PARALLEL} GOFLAGS="-p=${GO_PARALLEL}"
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY go.* ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w -linkmode external -extldflags '-static'" -o /out/project-scientist ./cmd/project-scientist

FROM golang:1.26-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648 AS test
ARG GO_PARALLEL=2
ENV GOMAXPROCS=${GO_PARALLEL} GOFLAGS="-p=${GO_PARALLEL}"
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY go.* ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY fixtures ./fixtures
COPY web ./web
CMD ["go", "test", "-mod=readonly", "./..."]

FROM alpine:3.22@sha256:310c62b5e7ca5b08167e4384c68db0fd2905dd9c7493756d356e893909057601 AS runtime
LABEL org.opencontainers.image.source="https://github.com/warpapaya/ProjectScientist"       org.opencontainers.image.description="Project Scientist local lab-test prototype; not for customer/prod data"
RUN addgroup -S scientist && adduser -S scientist -G scientist && mkdir -p /data /app/web && chown -R scientist:scientist /data /app
WORKDIR /app
COPY --from=build /out/project-scientist /app/project-scientist
COPY --chown=scientist:scientist web ./web
COPY --chown=scientist:scientist fixtures ./fixtures
USER scientist
ENV PSC_ADDR=:8080 PSC_DATA_DIR=/data
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
CMD ["/app/project-scientist"]
