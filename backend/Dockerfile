# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -o /migrate ./migrations

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /api /migrate /app/
EXPOSE 8080
CMD ["/app/api"]
