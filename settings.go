package main

import (
	"benchai/qlite/server"
	"gopkg.in/yaml.v3"
	"io"
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
	MaxSubscriberConnections uint16 `yaml:"maxSubscriberConnections"`
	MaxPublisherConnections  uint16 `yaml:"maxPublisherConnections"`
	MaxMessages              uint32 `yaml:"maxMessages"`
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

	serv, err := server.NewServer(users, s.Port, s.MaxSubscriberConnections, s.MaxPublisherConnections, s.MaxMessages)

	if err != nil {
		return nil, err
	}

	return serv, nil
}

func loadServerSettings(file os.File) (*server.Server, error) {
	content, err := io.ReadAll(&file)

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
