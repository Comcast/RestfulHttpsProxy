FROM golang:alpine
RUN apk update && apk add --virtual build-dependencies build-base git
ENV GOPATH /go
RUN mkdir -p /src/restfulHttpsProxy
ADD . /src/restfulHttpsProxy
WORKDIR /src/restfulHttpsProxy
EXPOSE 9999
EXPOSE 9998
CMD ["make"]
