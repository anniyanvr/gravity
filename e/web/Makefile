LOCALDIR := $(dir $(CURDIR)/$(word $(words $(MAKEFILE_LIST)),$(MAKEFILE_LIST)))
REPO_VER_TAG := $(shell git describe --tags --abbrev=8)
CONTAINER_DIST_DIR=/app/dist
# CONTAINER_NAME needs to be unique to accommodate parallel builds
CONTAINER_NAME := gravity-web-container-$(REPO_VER_TAG)-$(shell bash -c 'echo $$RANDOM')
IMAGE_NAME := gravity-web:1.0.0

HOST_DIR="$$(pwd)"
HOST_TMP_VOL:= $(shell mktemp -d)

# directory where to copy oss source code

OSS_DIR := $(HOST_DIR)/oss-src
SHARED_DIR := $(HOST_DIR)/shared
OSS_BRANCH_DIR=$(HOST_DIR)/../../web

.PHONY:all
all: docker-build

##############################
#   Docker Targets
##############################

#
# Builds dist files
#
.PHONY:docker-build
docker-build: ensure-deps
	rm -rf $(HOST_DIR)/dist
	DOCKER_BUILDKIT=1 PROGRESS_NO_TRUNC=1 docker build --force-rm=true -t $(IMAGE_NAME) .
	docker create --name $(CONTAINER_NAME) -t $(IMAGE_NAME) && \
	docker cp $(CONTAINER_NAME):$(CONTAINER_DIST_DIR) $(HOST_DIR)
	docker rm $(CONTAINER_NAME) | true

#
# Removes the local Docker container
#
.PHONY: docker-clean
docker-clean:
	docker rmi --force $(IMAGE_NAME)

#
# 'make docker-enter' builds a Docker container with a website
#
.PHONY:docker-enter
docker-enter:
	docker run -ti --rm=true -t $(IMAGE_NAME) /bin/bash

##############################
#   Native Localhost
##############################

.PHONY:ensure-deps
ensure-deps:
	 rm -rf $(OSS_DIR)
	 rm -rf $(SHARED_DIR)
	 mkdir -p $(OSS_DIR)/
	 mkdir -p $(SHARED_DIR)/
	 cp -R $(OSS_BRANCH_DIR)/src/. $(OSS_DIR)/
	 cp -R $(OSS_BRANCH_DIR)/shared/. $(SHARED_DIR)/

#
# locally installs npm packages and builds
#
.PHONY:local
local:
	npm install
	npm run build

#
# 'make run' starts a local debugging environment
#
.PHONY:run
run:
	npm run start

.PHONY:clean
clean:
	rm -rf dist
	rm -rf node_modules
	rm -rf $(OSS_DIR)
	rm -rf $(SHARED_DIR)
