FROM golang:alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/bin

FROM alpine:3.20 AS runtime
RUN adduser -D -H app
WORKDIR /app
COPY --from=builder /app/bin ./bin
COPY templates ./templates
USER app
EXPOSE 8080
ENTRYPOINT ["./bin"]
