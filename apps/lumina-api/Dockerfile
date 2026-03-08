FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/lumina-api ./cmd/gateway

FROM alpine:3.22

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /out/lumina-api /usr/local/bin/lumina-api

EXPOSE 8080
ENV PORT=8080

ENTRYPOINT ["/usr/local/bin/lumina-api"]
