FROM golang

RUN go get code.google.com/p/go-uuid/uuid
RUN go get github.com/fsouza/go-dockerclient
RUN go get github.com/golang/glog
RUN go get github.com/gorilla/mux
RUN go get github.com/gorilla/websocket

ADD . /go/src/github.com/ortutay/dlogs
RUN go install github.com/ortutay/dlogs

ADD templates /templates
ADD static /static

ENTRYPOINT /go/bin/dlogs -alsologtostderr -templates_path=/templates -static_dir=/

EXPOSE 8080