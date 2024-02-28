FROM golang:1.21-alpine as builder
ENV GOOS=linux
WORKDIR /src
COPY go.sum go.sum
COPY go.mod go.mod
RUN go mod download
COPY main.go main.go
RUN go build .

FROM alpine:3
ENV AIRFLOW_USER 50000
WORKDIR /app
RUN adduser -u ${AIRFLOW_USER} airflow -D
USER ${AIRFLOW_USER}
COPY --from=builder /src/knaudit /app/knaudit
CMD ["/app/knaudit"]
