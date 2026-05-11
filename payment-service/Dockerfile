FROM golang:1.25-alpine AS build
RUN apk add --no-cache git
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/payment-service ./cmd/payment-service

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
COPY --from=build /out/payment-service /payment-service
EXPOSE 50051 8081
ENTRYPOINT ["/payment-service"]
