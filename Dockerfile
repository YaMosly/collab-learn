FROM golang:1.27.3-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /collablearn ./cmd/server

FROM alpine:3.22

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /collablearn ./collablearn
COPY --from=builder /app/web ./web
USER 10001:10001

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=4s --start-period=60s --retries=3 CMD ["/app/collablearn", "healthcheck"]
ENTRYPOINT ["/app/collablearn"]
