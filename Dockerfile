# build stage
FROM golang:alpine AS build-env
# need to have git installed
ENV GOPATH=/go
RUN apk add --no-cache git
ADD . /go/src/app
RUN cd /go/src/app && go get && go build -o streams-to-ws

# final stage
FROM alpine
WORKDIR /app
COPY --from=build-env /go/src/app/streams-to-ws /app/
ENTRYPOINT ./streams-to-ws
