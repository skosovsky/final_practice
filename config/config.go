package config

import (
	"context"
	"os"
)

type AppConfig struct {
	KafkaBootstrapServers string
	TopicName             string
}

func NewAppConfig(_ context.Context) *AppConfig {
	bootstrapServers := getEnv("KAFKA_BOOTSTRAP_SERVERS", "127.0.0.1:9094,127.0.0.1:9095,127.0.0.1:9096")
	topicName := getEnv("KAFKA_TOPIC_NAME", "products")

	return &AppConfig{
		KafkaBootstrapServers: bootstrapServers,
		TopicName:             topicName,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return defaultValue
}
