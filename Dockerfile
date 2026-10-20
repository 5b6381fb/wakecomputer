# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS build
WORKDIR /src

ENV CGO_ENABLED=0 GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

ARG VERSION=0.1.0
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ARG TARGETARCH=amd64

RUN go build -trimpath -ldflags "-s -w \
  -X github.com/wakecomputer/wakecomputer/internal/version.Version=${VERSION} \
  -X github.com/wakecomputer/wakecomputer/internal/version.Commit=${COMMIT} \
  -X github.com/wakecomputer/wakecomputer/internal/version.BuildTime=${BUILD_TIME}" \
  -o /out/wakecomputer ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/wakecomputer /app/wakecomputer
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

USER nonroot:nonroot
EXPOSE 8085
ENTRYPOINT ["/app/wakecomputer"]
