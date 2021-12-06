package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gorilla/websocket"
)

var (
	ctx      = context.Background()
	addr     = flag.String("addr", ":8080", "http service address")
	upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	dataTempl = template.Must(template.New("").Parse(dataHTML))
)

const (
	// Poll file for changes with this period.
	pollPeriod = 100 * time.Millisecond
	// Time allowed to write the file to the client.
	writeWait = 10 * time.Second
)

type Streams struct {
	rdb *redis.Client
}

func readStream(lastMod time.Time, rdb *redis.Client, stream string) ([]byte, time.Time, error) {
	var valmaps []map[string]interface{}
	res, _ := rdb.XRead(ctx,
		&redis.XReadArgs{
			Streams: []string{stream, strconv.FormatInt(lastMod.UnixMilli(), 10)},
			Block:   pollPeriod},
	).Result()

	if len(res) == 0 {
		return nil, time.Now(), nil
	}

	for _, r := range res {
		for _, j := range r.Messages {
			valmaps = append(valmaps, j.Values)
		}
	}

	jsonByte, err := json.Marshal(valmaps)
	if err != nil {
		return []byte("parseError"), time.Now(), err
	}

	return jsonByte, time.Now(), nil
}

func writer(ws *websocket.Conn, lastMod time.Time, rdb *redis.Client, stream string) {
	lastError := ""
	fileTicker := time.NewTicker(pollPeriod)
	defer func() {
		fileTicker.Stop()
		ws.Close()
	}()
	for {
		select {
		case <-fileTicker.C:
			var p []byte
			var err error

			p, lastMod, err = readStream(lastMod, rdb, stream)

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

func (stream *Streams) serveWs(w http.ResponseWriter, r *http.Request) {
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

	s := r.FormValue("Stream")
	if s == "" {
		s = "default_stream"
	}

	go writer(ws, lastMod, stream.rdb, s)
}

func (stream *Streams) serveTest(w http.ResponseWriter, r *http.Request) {
	s := "test_stream"
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	p, lastMod, err := readStream(time.Unix(0, 0), stream.rdb, s)
	if err != nil {
		p = []byte(err.Error())
		lastMod = time.Unix(0, 0)
	}
	var v = struct {
		Host    string
		Data    string
		LastMod string
		Stream  string
	}{
		r.Host,
		string(p),
		strconv.FormatInt(lastMod.UnixNano(), 16),
		s,
	}
	dataTempl.Execute(w, &v)
}

func main() {
	redisClient := redis.NewClient(&redis.Options{
		Password:        "",
		Addr:            fmt.Sprintf("%s:%d", "localhost", 6379),
		DB:              0,
		MinIdleConns:    1,                    // make sure there are at least this many connections
		MinRetryBackoff: 8 * time.Millisecond, //minimum amount of time to try and backupf
		MaxRetryBackoff: 5000 * time.Millisecond,
		MaxConnAge:      0,  //3 * time.Second this will cause everyone to reconnect every 3 seconds - 0 is keep open forever
		MaxRetries:      10, // retry 10 times : automatic reconnect if a proxy is killed
		IdleTimeout:     time.Second,
	})
	streams := &Streams{rdb: redisClient}
	http.HandleFunc("/ws", streams.serveWs)
	http.HandleFunc("/test", streams.serveTest)
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
                var conn = new WebSocket("ws://{{.Host}}/ws?lastMod={{.LastMod}}&Stream={{.Stream}}");
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
