FROM golang:alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY . .
RUN CGO_ENABLED=1 CGO_CFLAGS="-D_LARGEFILE64_SOURCE" GOOS=linux GOARCH=arm64 go build -a -installsuffix cgo ./cli/rpc

FROM ghcr.io/intersectmbo/cardano-node:9.2.0 AS source
FROM alpine AS runner
RUN apk add jq
COPY --from=source /nix/store /nix/store
COPY --from=builder /app/rpc /usr/local/bin/cardano-rpc
RUN ln -s /nix/store/5vil862qi69xxw3fii620cpnw1j4fhwj-cardano-cli-exe-cardano-cli-9.4.1.0/bin/cardano-cli /usr/local/bin/
RUN ln -s /nix/store/gnfinsxkh8y98ylmafww34687wfqnflv-cardano-node-exe-cardano-node-9.2.0/bin/cardano-node /usr/local/bin/
