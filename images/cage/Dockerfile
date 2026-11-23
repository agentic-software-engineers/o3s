# NAME
#        Dockerfile — the cage image every home's dev container starts from
#
# DESCRIPTION
#        A pinned Ubuntu with the everyday tools of a dev container, the allowlist's markers
#        loaded into every shell, one PATH entry per directory, and the system's CA bundle as
#        the trust store of every runtime, so the proxy's certificates hold everywhere.
#
# USAGE
#        Built from the repository's root, which the bridge's Go source needs:
#        docker build -f images/cage/Dockerfile .
#
# SEE ALSO
#        dedupe-path.sh, compose.yaml, cmd/o3s-cage, .github/workflows/publish.yaml

# The bridge that carries the cage's loopback to the host's sockets, built static for any base
FROM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS bridge
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/o3s-cage ./cmd/o3s-cage
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags=-s -o /out/o3s-cage ./cmd/o3s-cage

FROM ubuntu:24.04@sha256:73b999393d07f2bf65a40373778a44dc9a6d54ad7718d29247d8ff9413c4d6a8

# Predictable UTF-8 locale
ENV LANG=C.UTF-8

# Base packages
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
      git \
      iproute2 \
      less \
      openssh-client \
      vim \
  && rm -rf /var/lib/apt/lists/*

# JSON parser for the policy
COPY --from=mikefarah/yq:4.54.3@sha256:7834187596ad56fd5a81436114e0b0b9dd54c91bb43946d1f7145b2be5f9c3d2 /usr/bin/yq /usr/local/bin/yq

# Load the markers into every interactive shell
RUN printf '\n# Export the markers the allowlist declares\nif [ -f /etc/profile.d/o3s-secrets.sh ]; then . /etc/profile.d/o3s-secrets.sh; fi\n' \
      >> /etc/bash.bashrc

# Fold the PATH prepends of every profile script into one entry per directory
COPY images/cage/dedupe-path.sh /usr/local/share/o3s/dedupe-path.sh
RUN printf '\n. /usr/local/share/o3s/dedupe-path.sh\n' >> /home/ubuntu/.profile

# The bridge to the host's sockets, on every shell's PATH
COPY --from=bridge /out/o3s-cage /usr/local/bin/o3s-cage

# Point runtimes with their own trust store at the system CA bundle
ENV NODE_EXTRA_CA_CERTS=/etc/ssl/certs/ca-certificates.crt \
    REQUESTS_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt \
    SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt \
    GIT_SSL_CAINFO=/etc/ssl/certs/ca-certificates.crt

RUN mkdir -p /home/ubuntu/features /home/ubuntu/projects \
 && chown -R ubuntu:ubuntu /home/ubuntu
