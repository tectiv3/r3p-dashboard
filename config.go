package main

import (
	"encoding/json"
	"os"
)

type MQTTConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	TopicPrefix string `json:"topic_prefix"`
}

type Config struct {
	MQTT   MQTTConfig `json:"mqtt"`
	DBPath string     `json:"db_path"`
	Listen string     `json:"listen"`
}

func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		MQTT: MQTTConfig{
			Host:        "localhost",
			Port:        1883,
			TopicPrefix: "ecoflow/r3p",
		},
		DBPath: "database.db",
		Listen: ":8080",
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
