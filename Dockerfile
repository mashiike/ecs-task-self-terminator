FROM golang:1.21-alpine3.18 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o ecs-task-self-terminator /app

FROM alpine:3.18
LABEL maintainer "mashiike <m.ikeda0901@gmail.com>"

RUN apk --no-cache add ca-certificates
COPY --from=builder /app/ecs-task-self-terminator /usr/local/bin/ecs-task-self-terminator
WORKDIR /
ENTRYPOINT ["/usr/local/bin/ecs-task-self-terminator"]
