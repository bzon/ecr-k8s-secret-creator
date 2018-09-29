VERSION ?= latest
LDFLAGS = -ldflags "-X main.VERSION=$(VERSION) -X main.COMMIT=$(shell git rev-parse --short HEAD) -X main.BRANCH=$(shell git branch | grep \* | cut -d ' ' -f2)"

linux-build: dep
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ${LDFLAGS}
	chmod +x ecr-k8s-secret-creator
	file -s ecr-k8s-secret-creator

docker-build: linux-build
	docker build -t bzon/ecr-k8s-secret-creator:$(VERSION) .

push:
	docker push bzon/ecr-k8s-secret-creator:$(VERSION)

dep:
	go get k8s.io/kube-openapi/pkg/util/proto
	go get ./...

test:
	go test -v ./...

coverage:
	go test -cpu=1 -v ./... -failfast -coverprofile=coverage.txt -covermode=count
