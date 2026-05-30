help:
	@echo "Available targets:"
	@echo "build - build the Go binary"
	@echo "run - run the Go binary with environment variables from .env"
	@echo "docker-build - build the Docker image"
	@echo "docker-run - run the Docker container with environment variables from .env"
	@echo "clean - remove the built binary and Docker image"

.PHONY: help build run docker-build docker-run clean

build:
	go build -o server ./cmd/server

run:
	export $(cat .env | xargs) && ./server

docker-build:
	docker build -t server .

docker-run:
	docker run -p 8080:8080 --env-file .env server

clean:
	rm -f server
	docker rmi server || true
