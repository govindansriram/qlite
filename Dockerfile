FROM alpine:latest

RUN apk update && apk upgrade

WORKDIR /app

COPY ./qlite qlite

COPY ./config.yaml config.yaml

EXPOSE 8080

CMD ["./qlite", "start", "config.yaml"]