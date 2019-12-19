VERSION := $(shell git rev-parse --short HEAD)
BUILDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build -v $(BUILDFLAGS)

serve_assets:
	docker run --rm -p 8081:80 \
		-v ${PWD}/node_modules:/usr/share/nginx/html/assets \
		-v ${PWD}/static:/usr/share/nginx/html/static \
		-v ${PWD}/tests/docker/default.conf:/etc/nginx/conf.d/default.conf:ro \
		nginx
