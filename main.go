package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	argsWithoutProg := os.Args[1:]

	if len(argsWithoutProg) != 2 {
		log.Fatal("invalid command, command must be 'start' followed by config path")
	}

	if argsWithoutProg[0] != "start" {
		log.Fatalf("command %s is not a valid command", argsWithoutProg[0])
	}

	fileInfo, err := os.Stat(argsWithoutProg[1])
	if err != nil {
		log.Fatal(err)
	}

	if fileInfo.IsDir() {
		log.Fatal("required file received directory")
	}

	file, err := os.Open(argsWithoutProg[1])
	if err != nil {
		log.Fatal(err)
	}

	server, err := loadServerSettings(file)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		server.Stop()
	}()

	server.Start()
}
