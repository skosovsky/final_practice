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

	if err := initKafkaTopic(ctx, appConfig); err != nil {
		log.Fatal(err)
	}

	go func() {
		if err := startProducer(ctx, appConfig); err != nil {
			log.Print(err)
		}
	}()

	go func() {
		if err := startSingleMessageConsumer(ctx, appConfig); err != nil {
			log.Print(err)
		}
	}()

	go func() {
		if err := startBatchMessageConsumer(ctx, appConfig); err != nil {
			log.Print(err)
		}
	}()

	<-ctx.Done()
}

// bin/kafka-topics.sh --create --topic products --bootstrap-server localhost:9092 --partitions 3 --replication-factor 2
func initKafkaTopic(ctx context.Context, appConfig *config.AppConfig) error {
	const timeout = 60 * time.Second

	kafkaAdminConfig := &kafka.ConfigMap{"bootstrap.servers": appConfig.KafkaBootstrapServers}
	kafkaAdminClient, err := kafka.NewAdminClient(kafkaAdminConfig)
	if err != nil {
		return fmt.Errorf("failed to create kafka admin client: %w", err)
	}
	defer kafkaAdminClient.Close()

	metadata, err := kafkaAdminClient.GetMetadata(&appConfig.TopicName, false, int(timeout))
	if err != nil {
		return fmt.Errorf("failed to get metadata: %w", err)
	}

	// exit if a topic exists
	if _, exists := metadata.Topics[appConfig.TopicName]; exists {
		return nil
	}

	results, err := kafkaAdminClient.CreateTopics(
		ctx,
		[]kafka.TopicSpecification{
			{
				Topic:             appConfig.TopicName,
				NumPartitions:     3,
				ReplicationFactor: 2,
			},
		},
		kafka.SetAdminOperationTimeout(timeout),
	)
	if err != nil {
		return fmt.Errorf("failed to create topic: %w", err)
	}

	for _, result := range results {
		log.Printf("Created topic %s\n", result.Topic)
	}

	return nil
}

type Product struct {
	Name  string `json:"name"`
	Price int    `json:"price"`
}

func startProducer(ctx context.Context, appConfig *config.AppConfig) error {
	configKafka := &kafka.ConfigMap{"bootstrap.servers": appConfig.KafkaBootstrapServers}

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
				value := Product{}

				err = json.Unmarshal(event.Value, &value)
				if err != nil {
					log.Printf("failed to unmarshal payload: %v\n", err)
				}

				fmt.Printf("Message in a topic %s:\n%+v\n", event.TopicPartition, value)

				if event.Headers != nil {
					fmt.Printf("Headers: %v\n", event.Headers)
				}
			case kafka.Error:
				log.Printf("error: %v: %v\n", event.Code(), event)
			default:
				fmt.Printf("Another events %v\n", event)
			}
		}
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
			return ctx.Err()
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
			default:
				fmt.Printf("Another events %v\n", event)
			}
		}
	}
}

func processBatch(batch []*kafka.Message) {
	for _, event := range batch {
		value := Product{}

		err := json.Unmarshal(event.Value, &value)
		if err != nil {
			log.Printf("failed to unmarshal payload: %v\n", err)
			continue
		}

		fmt.Printf("Message in a topic %s:\n%+v\n", event.TopicPartition, value)

		if event.Headers != nil {
			fmt.Printf("Headers: %v\n", event.Headers)
		}
	}
}
