.PHONY: build clean test zip

BINARY_NAME=bootstrap
ZIP_NAME=function.zip

build:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -ldflags="-s -w" -o $(BINARY_NAME) .

zip: build
	zip $(ZIP_NAME) $(BINARY_NAME)

test:
	go test -v ./...

clean:
	rm -f $(BINARY_NAME) $(ZIP_NAME)
