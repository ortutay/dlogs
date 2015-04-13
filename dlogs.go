package main

import (
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"code.google.com/p/go-uuid/uuid"

	"github.com/fsouza/go-dockerclient"
	log "github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var (
	port           = flag.String("port", "8080", "Port to listen on")
	staticDir      = flag.String("static_dir", ".", "Path to static files")
	templatesPath  = flag.String("templates_path", "templates", "Path to templates")
	dockerEndpoint = flag.String("docker_endpoint", "unix:///var/run/docker.sock", "Docker API endpoint")
)

// Store some log lines in a buffer, so we can send them to clients when they
// connect
const logsBufferSize = 250

var logsBuffer []*string

func main() {
	flag.Parse()

	r := mux.NewRouter()
	mux := http.NewServeMux()

	r.Handle("/", Endpoint{Serve: handleHome})
	r.Handle("/logs", Endpoint{Serve: handleLogsStream})
	mux.Handle("/", r)

	http.Handle("/static/", http.FileServer(http.Dir(*staticDir)))
	http.Handle("/", r)

	go dockerLogStream(*dockerEndpoint)

	log.Infof("Listening at %v...", *port)
	if err := http.ListenAndServe(":"+*port, nil); err != nil {
		log.Fatal(err)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request, ctx *Context) error {
	file, err := os.Open(fmt.Sprintf("%s/dlogs.html", *templatesPath))
	if err != nil {
		return err
	}
	buf, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	tmpl := template.Must(template.New("nodez").Parse(string(buf)))
	tc := make(map[string]interface{})
	if err := tmpl.Execute(w, tc); err != nil {
		return err
	}

	return nil
}

var msgChans = make(map[string]chan string)
var msgChansLock sync.Mutex

func handleLogsStream(w http.ResponseWriter, r *http.Request, ctx *Context) error {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}

	msgChansLock.Lock()
	id := uuid.New()
	ch := make(chan string, 10)
	msgChans[id] = ch
	defer func() {
		msgChansLock.Lock()
		delete(msgChans, id)
		close(ch)
		conn.Close()
		msgChansLock.Unlock()
	}()
	msgChansLock.Unlock()

	// Write the buffered log lines
	go func() {
		for _, line := range logsBuffer {
			time.Sleep(5 * time.Millisecond)
			ch <- *line
		}
	}()

	for {
		logData := <-ch
		if err := conn.WriteMessage(websocket.TextMessage, []byte(logData)); err != nil {
			return fmt.Errorf("couldn't write to websocket: %s", err)
		}
	}
}

func dockerLogStream(endpoint string) {
	client, err := docker.NewClient(endpoint)
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("docker client: %v", client)

	var container *docker.APIContainers
	for {
		containers, err := client.ListContainers(docker.ListContainersOptions{})
		if err != nil {
			log.Fatal(err)
		}

		log.Infof("Got containers: %v", containers)

		for _, c := range containers {
			// TODO: fix this obviously broken check
			if !strings.Contains(c.Image, "/dlogs") && !strings.Contains(c.Command, "/dlogs") {
				container = &c
				break
			}
		}
		if container != nil {
			break
		}
		time.Sleep(1 * time.Second)
	}

	log.Infof("Tracking container %s: %v", container.Image, container)

	// TODO: loop if container is nil

	err = client.Logs(docker.LogsOptions{
		Container:    container.ID,
		OutputStream: dockerLogReceiver{},
		Stdout:       true,
		Follow:       true,
		RawTerminal:  true,
		Tail:         strconv.Itoa(logsBufferSize),
	})
	if err != nil {
		log.Fatal(err)
	}
}

type dockerLogReceiver struct{}

func (d dockerLogReceiver) Write(p []byte) (n int, err error) {
	msgChansLock.Lock()
	s := removeNonUTF8(string(p))
	if len(logsBuffer) < logsBufferSize {
		logsBuffer = append(logsBuffer, &s)
	} else {
		logsBuffer = append(logsBuffer[1:], &s)
	}
	for _, ch := range msgChans {
		ch <- s
	}
	msgChansLock.Unlock()
	log.Info(s)
	return len(p), nil
}

// TODO(ortutay): something better here
// from http://stackoverflow.com/questions/20401873/remove-invalid-utf-8-characters-from-a-string-go-lang
func removeNonUTF8(s string) string {
	if !utf8.ValidString(s) {
		v := make([]rune, 0, len(s))
		for i, r := range s {
			if r == utf8.RuneError {
				_, size := utf8.DecodeRuneInString(s[i:])
				if size == 1 {
					continue
				}
			}
			v = append(v, r)
		}
		s = string(v)
	}
	return s
}
