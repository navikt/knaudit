FROM golang:1.22-alpine as builder
ENV GOOS=linux
WORKDIR /src
COPY go.sum go.sum
COPY go.mod go.mod
RUN go mod download
COPY main.go main.go
RUN go build .

FROM alpine:3
RUN adduser -u 50000 airflow -D
WORKDIR /app
COPY --from=builder /src/knaudit /app/knaudit
CMD ["/app/knaudit"]
