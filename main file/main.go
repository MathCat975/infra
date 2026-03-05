package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type CacheEntry struct {
	body      []byte
	expiresAt time.Time
}

var (
	port     int
	backends string
	timeout  int
	cacheTTL int
)

var backendList []string
var backendIndex int
var backendMu sync.Mutex

var cache = map[string]CacheEntry{}
var cacheMu sync.Mutex

func nextBackend() string {

	backendMu.Lock()
	defer backendMu.Unlock()

	b := backendList[backendIndex%len(backendList)]
	backendIndex++

	return b
}

func getCache(key string) ([]byte, bool) {

	cacheMu.Lock()
	defer cacheMu.Unlock()

	entry, ok := cache[key]

	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		delete(cache, key)
		return nil, false
	}

	return entry.body, true
}

func setCache(key string, body []byte) {

	cacheMu.Lock()
	defer cacheMu.Unlock()

	cache[key] = CacheEntry{
		body:      body,
		expiresAt: time.Now().Add(time.Duration(cacheTTL) * time.Second),
	}
}

func handler(w http.ResponseWriter, r *http.Request) {

	id := r.URL.Query().Get("id")

	if id == "" {
		http.Error(w, "missing id", 400)
		return
	}

	key := id

	if body, ok := getCache(key); ok {

		log.Println("cache hit id=", id)

		w.Header().Set("Content-Type", "text/html")
		w.Write(body)

		return
	}

	log.Println("cache miss id=", id)

	client := http.Client{
		Timeout: time.Duration(timeout) * time.Millisecond,
	}

	for i := 0; i < len(backendList); i++ {

		backend := nextBackend()

		url := fmt.Sprintf("%s/?id=%s", backend, id)

		resp, err := client.Get(url)

		if err != nil {
			log.Println("backend error:", backend, err)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			continue
		}

		setCache(key, body)

		w.Header().Set("Content-Type", "text/html")
		w.Write(body)

		log.Println("served from backend:", backend)

		return
	}

	http.Error(w, "all backends failed", 502)
}

func main() {

	flag.IntVar(&port, "port", 80, "server port")
	flag.StringVar(&backends, "backends", "", "backend list comma separated")
	flag.IntVar(&timeout, "timeout", 2000, "timeout ms")
	flag.IntVar(&cacheTTL, "cache-ttl", 10, "cache ttl seconds")

	flag.Parse()

	if backends == "" {
		log.Fatal("no backends defined")
	}

	backendList = strings.Split(backends, ",")

	http.HandleFunc("/", handler)

	addr := fmt.Sprintf(":%d", port)

	log.Println("main server running on", addr)
	log.Println("backends:", backendList)

	log.Fatal(http.ListenAndServe(addr, nil))
}
