
all:
	go build

dist: clean
	mkdir -p dist
	GOOS=linux GOARCH=amd64 go build -o dist/telegraf-input-lightware-linux-amd64
	GOOS=darwin GOARCH=arm64 go build -o dist/telegraf-input-lightware-darwin-arm64

clean:
	rm -rf dist/
	rm -f telegraf-input-lightware