package config

type Config struct {
	Peers []string
}

func GetConfig() Config {
	return Config{
		Peers: []string{
			"localhost:8001",
			"localhost:8002",
			"localhost:8003",
			"localhost:8004",
			"localhost:8005",
		},
	}
}
