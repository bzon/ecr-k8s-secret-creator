build:
	go get ./...
	GOOS=linux GOARCH=amd64 go build
	docker build -t bzon/ecr-k8s-secret-creator .
push:
	docker push bzon/ecr-k8s-secret-creator:latest
