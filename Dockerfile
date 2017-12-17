FROM golang:1.9

RUN go get -u github.com/golang/dep/cmd/dep && go install github.com/golang/dep/cmd/dep

WORKDIR /go/src/github.com/iced-mocha/twitter-client
COPY . /go/src/github.com/iced-mocha/twitter-client

RUN dep ensure -v && go install -v

ENTRYPOINT ["twitter-client"]
