# Собрать все приложения
build:
	go build -o bin/producer ./cmd/producer
	go build -o bin/consumer_single ./cmd/consumer_single
	go build -o bin/consumer_batch ./cmd/consumer_batch

# Запустить продюсера локально
run-producer:
	go run ./cmd/producer

# Запустить SingleMessageConsumer локально
run-consumer-single:
	go run ./cmd/consumer_single

# Запустить BatchMessageConsumer локально
run-consumer-batch:
	go run ./cmd/consumer_batch

# Проверить зависимости
deps:
	go mod tidy

# Запустить docker-compose
compose-up:
	docker compose up --build

# Остановить docker-compose
compose-down:
	docker compose down

# Полная пересборка с удалением томов (если надо сбросить все)
compose-reset:
	docker compose down -v
	docker compose up --build

# Очистить скомпилированные бинарники
clean:
	rm -rf bin/