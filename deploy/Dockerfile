# syntax=docker/dockerfile:1

# Production image: one self-contained static Go binary (marks + front page
# embedded). construct-server pins ghcr.io/einlanzerous/placard in
# versions.env. Build from the repo root:
#   docker build -f deploy/Dockerfile -t placard:latest .

# 1) Build the static binary. Pure Go => CGO disabled.
FROM golang:1.26-bookworm AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
# Build identity, reported by /healthz. Both default EMPTY, deliberately:
# an image built outside the publish workflow must not be able to claim it is
# a release, and internal/version maps empty to "dev" (the PRSR-32 rule).
# VERSION is bare semver, no "v" prefix; GIT_SHA the full 40-char commit.
ARG VERSION=
ARG GIT_SHA=
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w \
      -X github.com/Einlanzerous/placard/internal/version.Version=${VERSION} \
      -X github.com/Einlanzerous/placard/internal/version.Commit=${GIT_SHA}" \
    -o /out/placard ./cmd/placard

# 2) Runtime: slim Alpine with ca-certificates (HTTPS to jsDelivr for the
#    canonical-URL checks) and busybox wget for the compose healthcheck.
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates
COPY --from=build /out/placard /usr/local/bin/placard
ENV PLACARD_ADDR=":4009"
EXPOSE 4009
ENTRYPOINT ["placard"]
