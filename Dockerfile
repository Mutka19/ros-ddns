FROM golang:1.26-alpine AS builder
WORKDIR /app

COPY certs/*.crt /usr/local/share/ca-certificates/
RUN apk add --no-cache ca-certificates && update-ca-certificates

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o daemon ./cmd/

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/daemon /daemon
ENTRYPOINT ["/daemon"]
