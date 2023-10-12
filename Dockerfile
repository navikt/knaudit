FROM golang:1.21-alpine as builder
ENV GOOS=linux
WORKDIR /src
COPY go.sum go.sum
COPY go.mod go.mod
RUN go mod download
COPY main.go main.go
RUN go build .

FROM alpine:3
WORKDIR /app
COPY --from=builder /src/knaudit /app/knaudit
CMD ["/app/knaudit"]
