# Copyright 2019 Comcast Cable Communications Management, LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Go parameters
GOCMD=go
TARGET=restfulHttpsProxy
DOCKER_LOCAL_IMAGE=restful_https_proxy:local

EXPOSED_API_PORT=9998
PROXY_PORT=9999

run: build
				./$(TARGET) $(EXPOSED_API_PORT) $(PROXY_PORT)
build:
				$(GOCMD) build -o $(TARGET) -v
test:
				$(GOCMD) test -v ./...
clean:
				$(GOCMD) clean
				rm -f $(TARGET)

longTermDeploy: build
				./restartScript.sh

#Docker targets
docker-image:
				docker build -t $(DOCKER_LOCAL_IMAGE) \
				--build-arg EXPOSED_API_PORT=$(EXPOSED_API_PORT) \
				--build-arg PROXY_PORT=$(PROXY_PORT) .

docker-run:
				docker run -t \
				-p $(PROXY_PORT):$(PROXY_PORT) \
				-p $(EXPOSED_API_PORT):$(EXPOSED_API_PORT) \
				-i $(DOCKER_LOCAL_IMAGE)
