.PHONY: build clean test zip docker-build

BINARY_NAME=bootstrap
ZIP_NAME=function.zip
IMAGE_NAME=elasticache-auto-updater:latest

build:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags="-s -w" -o $(BINARY_NAME) .

zip: build
	zip $(ZIP_NAME) $(BINARY_NAME)

test:
	go test -v ./...

docker-build:
	docker build -t $(IMAGE_NAME) .

clean:
	rm -f $(BINARY_NAME) $(ZIP_NAME)
