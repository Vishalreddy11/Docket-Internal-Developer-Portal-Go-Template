.PHONY: build run tidy up down logs seed clean k8s-apply k8s-delete

build:
	go build -o bin/docket ./cmd/docket

run:
	go run ./cmd/docket

tidy:
	go mod tidy

up:
	docker compose up -d --build

down:
	docker compose down -v

logs:
	docker compose logs -f docket

seed:
	curl -X POST -H "X-API-Key: dev-secret-change-me" http://localhost:8080/seed?n=20

clean:
	rm -rf bin

k8s-apply:
	kubectl apply -f k8s/

k8s-delete:
	kubectl delete -f k8s/
