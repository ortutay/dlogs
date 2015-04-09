package main

import (
	"net/http"
	"reflect"
	"runtime"

	log "github.com/golang/glog"
	"github.com/gorilla/websocket"
)

var (
	upgrader = &websocket.Upgrader{
		// CheckOrigin:     func(r *http.Request) bool { return true },
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
)

type handler func(http.ResponseWriter, *http.Request, *Context) error

type Context struct {
}

type Endpoint struct {
	Serve handler
}

func (e Endpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := Context{}

	handlerName := runtime.FuncForPC(reflect.ValueOf(e.Serve).Pointer()).Name()

	log.Infof("%s %s [%s]", r.Method, r.URL.Path, handlerName)

	if err := e.Serve(w, r, &ctx); err != nil {
		log.Errorf("Error while executing %s: %v", handlerName, err)
	}
}
