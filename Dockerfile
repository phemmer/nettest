FROM google/golang:1.4

ENV APP_DIR $GOPATH/src/github.com/phemmer/nettest
WORKDIR $APP_DIR
CMD exec nettest
ADD . $APP_DIR/
RUN go get && go install
