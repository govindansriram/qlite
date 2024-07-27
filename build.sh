export GOOS=linux
export GOARCH=amd64
go build .

docker build -t qlite:latest .