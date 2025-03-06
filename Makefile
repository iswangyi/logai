.PHONY: build test clean run docker deploy help

BIN_DIR=bin
APP_NAME=logai
VERSION=1.0.0
TARGET?=all

help:
	@echo "可用命令:"
	@echo "  build [server|agent|all] - 编译指定组件"
	@echo "  docker [server|agent|all] - 构建指定组件镜像"
	@echo "  deploy [server|agent|all]  - 部署指定组件"
	@echo "  clean                    - 清理构建产物"

build:
	$(MAKE) build-server
	$(MAKE) build-agent

build-server:
	@echo "正在编译server组件..."
	CGO_ENABLED=0 go build -o $(BIN_DIR)/server ./cmd/server

build-agent:
	@echo "正在编译agent组件..."
	CGO_ENABLED=0 go build -o $(BIN_DIR)/agent ./cmd/agent

docker:
	$(MAKE) docker-server
	$(MAKE) docker-agent

docker-server:
	@echo "构建Server镜像..."
	docker build -t $(APP_NAME)-server:$(VERSION) \
		--build-arg TARGET=server -f deploy/Dockerfile.server .

docker-agent:
	@echo "构建Agent镜像..."
	docker build -t $(APP_NAME)-agent:$(VERSION) \
		--build-arg TARGET=agent -f deploy/Dockerfile.agent .

deploy:
	@if [ "$(filter-out all,$(TARGET))" = "server" ]; then \
		echo "部署Server组件..."; \
		kubectl apply -f deploy/server-deployment.yaml; \
	elif [ "$(filter-out all,$(TARGET))" = "agent" ]; then \
		echo "部署Agent组件..."; \
		kubectl apply -f deploy/agent-daemonset.yaml; \
	else \
		echo "部署所有组件..."; \
		kubectl apply -f deploy/server-deployment.yaml; \
		kubectl apply -f deploy/agent-daemonset.yaml; \
	fi


test:
	@echo "正在运行测试..."
	go test -v ./...

clean:
	@echo "清理构建产物..."
	@rm -rf $(BIN_DIR)

run:
	@echo "启动服务..."
	@$(BIN_DIR)/server


docker-push:
	@echo "推送镜像到仓库..."
	docker push $(APP_NAME):$(VERSION)
