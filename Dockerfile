FROM ubuntu:oracular-20240617

RUN apt-get update

WORKDIR /app

COPY ./qlite qlite

COPY ./config.yaml config.yaml

EXPOSE 8080

CMD ["./qlite", "start", "config.yaml"]