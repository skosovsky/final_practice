package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"final_practice/config"

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

	if err := startProducer(ctx, appConfig); err != nil {
		log.Print(err)
	}
}

type Product struct {
	Name  string `json:"name"`
	Price int    `json:"price"`
}

func startProducer(ctx context.Context, appConfig *config.AppConfig) error {
	configKafka := &kafka.ConfigMap{
		"bootstrap.servers": appConfig.KafkaBootstrapServers,
		"acks":              "all",
		"retries":           5,
	}

	producer, err := kafka.NewProducer(configKafka)
	if err != nil {
		return fmt.Errorf("failed to create producer: %w", err)
	}

	log.Printf("Producer created %v\n", producer)

	delivery := make(chan kafka.Event)

	go func() {
		for event := range delivery {
			msg := event.(*kafka.Message)

			if msg.TopicPartition.Error != nil {
				fmt.Printf("Error delivery message: %v\n", msg.TopicPartition.Error)
			} else {
				fmt.Printf("Message sent to topic %s [%d] offset %v\n",
					*msg.TopicPartition.Topic, msg.TopicPartition.Partition, msg.TopicPartition.Offset)
			}
		}
	}()

	go func() {
		if err = produce(ctx, appConfig, producer, delivery); err != nil {
			log.Print(err)
		}
	}()

	<-ctx.Done()

	producer.Flush(15000)
	producer.Close()
	close(delivery)

	return nil
}

func produce(ctx context.Context, appConfig *config.AppConfig, producer *kafka.Producer, delivery chan kafka.Event) error {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			value := &Product{
				Name:  fmt.Sprintf("product %d", rand.Int()),
				Price: rand.Int(),
			}

			payload, err := json.Marshal(value)
			if err != nil {
				log.Printf("failed to marshal payload: %v\n", err)
				continue
			}

			message := &kafka.Message{
				TopicPartition: kafka.TopicPartition{
					Topic:     &appConfig.TopicName,
					Partition: kafka.PartitionAny,
				},
				Value: payload,
				Headers: []kafka.Header{
					{
						Key:   "myTestHeader",
						Value: []byte("header values are binary"),
					},
				},
			}

			err = producer.Produce(message, delivery)
			if err != nil {
				log.Printf("failed to produce message: %v\n", err)
				continue
			}
		}
	}
}
