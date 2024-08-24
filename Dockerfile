FROM alpine:latest

RUN apk update && apk upgrade

RUN mkdir -p "/app/config"

WORKDIR /app

COPY ./qlite qlite

CMD ["./qlite", "start", "config/config.yaml"]