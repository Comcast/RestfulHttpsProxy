FROM golang:alpine
RUN apk update && apk add --virtual build-dependencies build-base git
ARG EXPOSED_API_PORT=9998
ARG PROXY_PORT=9999
ENV A_PORT ${EXPOSED_API_PORT}
ENV P_PORT ${PROXY_PORT}
ENV GOPATH /go
RUN mkdir -p /src/restfulHttpsProxy
ADD . /src/restfulHttpsProxy
WORKDIR /src/restfulHttpsProxy
EXPOSE ${EXPOSED_API_PORT}
EXPOSE ${PROXY_PORT}
CMD make EXPOSED_API_PORT=${A_PORT} PROXY_PORT=${P_PORT}
