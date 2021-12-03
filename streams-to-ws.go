package main

import (
	"flag"
	"log"
	"net/http"
	"strconv"
	"time"

	"html/template"

	"github.com/gorilla/websocket"
)

var (
	addr     = flag.String("addr", ":8080", "http service address")
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	dataTempl = template.Must(template.New("").Parse(dataHTML))
)

const (
	// Poll file for changes with this period.
	filePeriod = 100 * time.Millisecond
	// Time allowed to write the file to the client.
	writeWait = 10 * time.Second
)

func readStream() ([]byte, time.Time, error) {
	var p []byte
	//var err error
	lastMod := time.Now()

	//p, err = readFile()

	//if err != nil {
	//	return p, lastMod, err
	//}
	p = []byte(lastMod.String())

	return p, lastMod, nil
}

func writer(ws *websocket.Conn, lastMod time.Time) {
	lastError := ""
	fileTicker := time.NewTicker(filePeriod)
	defer func() {
		fileTicker.Stop()
		ws.Close()
	}()
	for {
		select {
		case <-fileTicker.C:
			var p []byte
			var err error

			p, lastMod, err = readStream()

			if err != nil {
				if s := err.Error(); s != lastError {
					lastError = s
					p = []byte(lastError)
				}
			} else {
				lastError = ""
			}

			if p != nil {
				ws.SetWriteDeadline(time.Now().Add(writeWait))
				if err := ws.WriteMessage(websocket.TextMessage, p); err != nil {
					return
				}
			}
		}
	}
}

func serveWs(w http.ResponseWriter, r *http.Request) {
	var lastMod time.Time
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if _, ok := err.(websocket.HandshakeError); !ok {
			log.Println(err)
		}
		return
	}

	if n, err := strconv.ParseInt(r.FormValue("lastMod"), 16, 64); err == nil {
		lastMod = time.Unix(0, n)
	}

	go writer(ws, lastMod)
}

func serveTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	p, lastMod, err := readStream()
	if err != nil {
		p = []byte(err.Error())
		lastMod = time.Unix(0, 0)
	}
	var v = struct {
		Host    string
		Data    string
		LastMod string
	}{
		r.Host,
		string(p),
		strconv.FormatInt(lastMod.UnixNano(), 16),
	}
	dataTempl.Execute(w, &v)
}

func main() {
	http.HandleFunc("/ws", serveWs)
	http.HandleFunc("/test", serveTest)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}

const dataHTML = `<!DOCTYPE html>
<html lang="en">
    <head>
        <title>WebSocket Example</title>
    </head>
    <body>
        <pre id="fileData">{{.Data}}</pre>
        <script type="text/javascript">
            (function() {
                var data = document.getElementById("fileData");
                var conn = new WebSocket("ws://{{.Host}}/ws?lastMod={{.LastMod}}");
                conn.onclose = function(evt) {
                    data.textContent = 'Connection closed';
                }
                conn.onmessage = function(evt) {
                    data.textContent = evt.data;
                }
            })();
        </script>
    </body>
</html>
`
