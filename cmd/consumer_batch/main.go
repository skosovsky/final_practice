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

	if err := startBatchMessageConsumer(ctx, appConfig); err != nil {
		log.Print(err)
	}
}

func startBatchMessageConsumer(ctx context.Context, appConfig *config.AppConfig) error {
	const timeoutMs = 500

	configKafka := kafka.ConfigMap{
		"bootstrap.servers":  appConfig.KafkaBootstrapServers,
		"group.id":           "consumer_batch",
		"session.timeout.ms": 6000,
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": false,
		"fetch.min.bytes":    600,
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

	var batch []*kafka.Message

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("interrupt signal: %w", ctx.Err())
		default:
			eventKafka := consumer.Poll(timeoutMs)
			if eventKafka == nil {
				continue
			}

			switch event := eventKafka.(type) {
			case *kafka.Message:
				batch = append(batch, event)

				if len(batch) >= 10 {
					processBatch(batch)

					_, err = consumer.CommitMessage(batch[len(batch)-1])
					if err != nil {
						log.Printf("failed to commit message: %v\n", err)
					}

					batch = []*kafka.Message{}
				}

			case kafka.Error:
				log.Printf("error: %v: %v\n", event.Code(), event)
			}
		}
	}
}

func processBatch(batch []*kafka.Message) {
	for _, event := range batch {
		value := model.Product{}

		err := json.Unmarshal(event.Value, &value)
		if err != nil {
			log.Printf("failed to unmarshal payload: %v\n", err)
			continue
		}

		fmt.Printf("[B] Message in a topic %s:\n%+v\n", event.TopicPartition, value)
	}
}
