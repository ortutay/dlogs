package main

import (
	"unicode/utf8"
	"sync"
	"strings"
	"flag"
	"net/http"
	"os"
	"fmt"
	"html/template"
	"io/ioutil"

	"code.google.com/p/go-uuid/uuid"

	"github.com/fsouza/go-dockerclient"
	log "github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var (
	port          = flag.String("port", "8080", "Port to listen on")
	staticDir     = flag.String("static_dir", ".", "Path to static files")
	templatesPath = flag.String("templates_path", "templates", "Path to templates")
	dockerHost = flag.String("docker_host", ":8888", "Docker host for log stream")
)

func main() {
	flag.Parse()

	r := mux.NewRouter()
	mux := http.NewServeMux()

	r.Handle("/", Endpoint{Serve: handleHome})
	r.Handle("/logs", Endpoint{Serve: handleLogsStream})
	mux.Handle("/", r)

	http.Handle("/static/", http.FileServer(http.Dir(*staticDir)))
	http.Handle("/", r)

	go dockerLogStream(*dockerHost)

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

	for {
		logData := <-ch
		if err := conn.WriteMessage(websocket.TextMessage, []byte(logData)); err != nil {
			return fmt.Errorf("couldn't write to websocket: %s", err)
		}
	}
}

func dockerLogStream(host string) {
	client, err := docker.NewClient(fmt.Sprintf("http://%s", host))
	if err != nil {
		log.Fatal(err)
	}
	log.Infof("docker client: %v", client)
	containers, err := client.ListContainers(docker.ListContainersOptions{})
	if err != nil {
		log.Fatal(err)
	}

	// TODO: flag to indicate which container to stream

	var container *docker.APIContainers
	for _, c := range containers {
		if !strings.HasPrefix(c.Image, "ortutay/dlogs") {
			container = &c
			break
		}
	}

	// TODO: loop if container is nil

	err = client.Logs(docker.LogsOptions{
		Container: container.ID,
		OutputStream: dockerLogReceiver{},
		Stdout: true,
		Follow: true,
		RawTerminal: true,
		Tail: "3",
	})
	if err != nil {
		log.Fatal(err)
	}
}

type dockerLogReceiver struct{}

func (d dockerLogReceiver) Write(p []byte) (n int, err error) {
	msgChansLock.Lock()
	s := removeNonUTF8(string(p))
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
