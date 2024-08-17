package main

import (
	"benchai/qlite/server"
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"os"
)

type user struct {
	Name       string `yaml:"name"`
	Password   string `yaml:"password"`
	Publisher  bool   `yaml:"publisher"`
	Subscriber bool   `yaml:"subscriber"`
}

type serverSettings struct {
	Users                    []user `yaml:"users"`
	Port                     uint16 `yaml:"port"`
	Address                  string `yaml:"address"`
	MaxSubscriberConnections uint16 `yaml:"maxSubscriberConnections"`
	MaxPublisherConnections  uint16 `yaml:"maxPublisherConnections"`
	MaxMessageSize           uint32 `yaml:"maxMessageSize"`
	MaxIoTimeSeconds         uint16 `yaml:"maxIoTimeSeconds"`
	MaxHiddenTimeSeconds     uint16 `yaml:"maxHiddenTimeSeconds"`
	MaxPollingTimeSeconds    uint16 `yaml:"maxPollingTimeSeconds"`
	Verbose                  bool   `yaml:"verbose"`
}

func (s *serverSettings) build() (*server.Server, error) {
	users := make([]server.User, len(s.Users))

	for idx, us := range s.Users {
		converted, err := server.NewUser(us.Name, us.Password, us.Publisher, us.Subscriber)

		if err != nil {
			return nil, err
		}

		users[idx] = *converted
	}

	serv, err := server.NewServer(
		users,
		s.Port,
		s.MaxSubscriberConnections,
		s.MaxPublisherConnections,
		s.MaxMessageSize,
		s.MaxIoTimeSeconds,
		s.MaxHiddenTimeSeconds,
		s.MaxPollingTimeSeconds,
		s.Address,
		s.Verbose)

	if err != nil {
		return nil, err
	}

	return serv, nil
}

func loadServerSettings(file *os.File) (*server.Server, error) {
	defer func() {
		err := file.Close()
		if err != nil {
			log.Println(err)
		}
	}()

	_, err := file.Seek(0, 0)
	if err != nil {
		return nil, err
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	settings := &serverSettings{}
	err = yaml.Unmarshal(content, settings)
	serv, err := settings.build()

	if err != nil {
		return nil, err
	}

	return serv, nil
}
