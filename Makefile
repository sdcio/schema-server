REMOTE_REGISTRY := registry.kmrd.dev/sdcio/schema-server
TAG := $(shell git describe --tags)
IMAGE := $(REMOTE_REGISTRY):$(TAG)
TEST_IMAGE := $(IMAGE)-test
USERID := 10000

# go versions
TARGET_GO_VERSION := go1.21.4
GO_FALLBACK := go
# We prefer $TARGET_GO_VERSION if it is not available we go with whatever go we find ($GO_FALLBACK)
GO_BIN := $(shell if [ "$$(which $(TARGET_GO_VERSION))" != "" ]; then echo $$(which $(TARGET_GO_VERSION)); else echo $$(which $(GO_FALLBACK)); fi)

build:
	mkdir -p bin
	CGO_ENABLED=0 ${GO_BIN} build -o bin/schemac client/main.go
	CGO_ENABLED=0 ${GO_BIN} build -o bin/schema-server main.go

test:
	robot tests/robot
	go test ./...

docker-build:
	ssh-add ./keys/id_rsa 2>/dev/null; true
	docker build --build-arg USERID=$(USERID) . -t $(IMAGE) --ssh default=$(SSH_AUTH_SOCK)

docker-push: docker-build
	docker push $(IMAGE)

release: docker-build
	docker tag $(IMAGE) $(REMOTE_REGISTRY):latest
	docker push $(REMOTE_REGISTRY):latest

docker-test:
	ssh-add ./keys/id_rsa 2>/dev/null; true
	docker build . -t $(TEST_IMAGE) -f tests/container/Dockerfile --ssh default=$(SSH_AUTH_SOCK)
	docker run -v ./tests/results:/results:rw $(TEST_IMAGE) robot --outputdir /results /app/tests/robot

run-distributed:
	./lab/distributed/run.sh build

run-combined:
	./lab/combined/run.sh build

stop:
	./lab/combined/stop.sh
	./lab/distributed/stop.sh
