FROM golang:1.23.3-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/server .
COPY config.json .
COPY credentials.json .
COPY web/ web/
COPY static/ static/

EXPOSE 8080
CMD ["./server"]
