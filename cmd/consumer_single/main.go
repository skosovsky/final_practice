package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"final_practice/config"
	"final_practice/model"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signals
		log.Println("Interrupt signal received, shutting down...")
		cancel()
	}()

	appConfig := config.NewAppConfig(ctx)

	if err := startSingleMessageConsumer(ctx, appConfig); err != nil {
		log.Print(err)
	}
}

func startSingleMessageConsumer(ctx context.Context, appConfig *config.AppConfig) error {
	const timeoutMs = 100

	configKafka := kafka.ConfigMap{
		"bootstrap.servers":  appConfig.KafkaBootstrapServers,
		"group.id":           "consumer_single",
		"session.timeout.ms": 6000,
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": true,
	}

	consumer, err := kafka.NewConsumer(&configKafka)
	if err != nil {
		return fmt.Errorf("failed to create consumer: %w", err)
	}
	defer consumer.Close()

	err = consumer.SubscribeTopics([]string{appConfig.TopicName}, nil)
	if err != nil {
		return fmt.Errorf("failed to subscribe to topic %s: %w", appConfig.TopicName, err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			eventKafka := consumer.Poll(timeoutMs)
			if eventKafka == nil {
				continue
			}

			switch event := eventKafka.(type) {
			case *kafka.Message:
				value := model.Product{}

				err = json.Unmarshal(event.Value, &value)
				if err != nil {
					log.Printf("failed to unmarshal payload: %v\n", err)
				}

				fmt.Printf("[S] Message in a topic %s:\n%+v\n", event.TopicPartition, value)
			case kafka.Error:
				log.Printf("error: %v: %v\n", event.Code(), event)
			}
		}
	}
}
