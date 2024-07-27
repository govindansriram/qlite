export GOOS=linux
export GOARCH=amd64
CGO_ENABLED=0 go build .

docker build -t qlite:latest .