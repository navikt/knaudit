FROM golang:1.20-alpine as builder
#RUN apk add --no-cache git make
ENV GOOS=linux
WORKDIR /src
COPY go.sum go.sum
COPY go.mod go.mod
RUN go mod download
COPY main.go main.go
RUN go build .

FROM alpine:3
WORKDIR /app
COPY --from=builder /src/knaudit /app/kaudit
CMD ["/app/kaudit"]
