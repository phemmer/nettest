FROM google/golang:1.4

ENV APP_DIR $GOPATH/src/github.com/phemmer/nettest
WORKDIR $APP_DIR
CMD exec nettest
EXPOSE 8080
ADD . $APP_DIR/
RUN go get && go install
