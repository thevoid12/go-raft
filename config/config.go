// config/config.go
package config

import "time"

type Config struct {
	Peers              []string
	HeartbeatInterval  time.Duration
	ElectionTimeoutMin time.Duration
	ElectionTimeoutMax time.Duration
}

// I didnt use a json or yaml or toml structure to add the configs
// configs can be taken out of go code if needed. I directly hardcoded it into a go file
func GetConfig() Config {
	return Config{
		Peers: []string{
			"localhost:8001",
			"localhost:8002",
			"localhost:8003",
			"localhost:8004",
			"localhost:8005",
		},
		// the minumun and max as well as heartbeatInterval are not the ideal times used widely it should be around 150-300 milli seconds
		HeartbeatInterval:  1 * time.Second,  // Leaders send heartbeats every 100ms
		ElectionTimeoutMin: 10 * time.Second, // Followers wait at least 10 sec minimum
		ElectionTimeoutMax: 15 * time.Second, //  // Followers wait at least 15 sec max
	}
}
