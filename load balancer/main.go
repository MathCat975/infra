package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Servers []string `yaml:"servers"`
}

var (
	port     = 80
	timeout  int
	cacheTTL int
)

var backendList []string
var backendIndex int
var backendMu sync.Mutex

type BackendStat struct {
	latency   time.Duration
	expiresAt time.Time
}

var backendStats = map[string]BackendStat{}
var backendStatsMu sync.Mutex

func loadConfig(path string) Config {

	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal("cannot read config:", err)
	}

	var cfg Config

	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		log.Fatal("cannot parse yaml:", err)
	}

	return cfg
}

func nextBackend() string {

	backendMu.Lock()
	defer backendMu.Unlock()

	b := backendList[backendIndex%len(backendList)]
	backendIndex++

	return b
}

func getBackendLatency(backend string) (time.Duration, bool) {
	backendStatsMu.Lock()
	defer backendStatsMu.Unlock()

	stat, ok := backendStats[backend]
	if !ok {
		return 0, false
	}

	if time.Now().After(stat.expiresAt) {
		delete(backendStats, backend)
		return 0, false
	}

	return stat.latency, true
}

func setBackendLatency(backend string, latency time.Duration) {
	backendStatsMu.Lock()
	defer backendStatsMu.Unlock()

	backendStats[backend] = BackendStat{
		latency:   latency,
		expiresAt: time.Now().Add(time.Duration(cacheTTL) * time.Second),
	}
}

func printBackendStats() {
	backendStatsMu.Lock()
	defer backendStatsMu.Unlock()

	log.Println("backend latency cache:")
	for backend, stat := range backendStats {
		log.Println(backend, stat.latency, "expiresAt", stat.expiresAt)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {

	printBackendStats()

	client := http.Client{
		Timeout: time.Duration(timeout) * time.Millisecond,
	}

	backends := make([]string, len(backendList))
	copy(backends, backendList)

	sort.Slice(backends, func(i, j int) bool {
		li, oki := getBackendLatency(backends[i])
		lj, okj := getBackendLatency(backends[j])

		if !oki {
			li = time.Hour
		}

		if !okj {
			lj = time.Hour
		}

		return li < lj
	})

	for i := 0; i < len(backends); i++ {

		backend := backends[i]

		url := fmt.Sprintf("http://%s/", backend)

		start := time.Now()

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

		latency := time.Since(start)
		setBackendLatency(backend, latency)

		w.Header().Set("Content-Type", "text/html")
		w.Write(body)

		log.Println("served from backend:", backend)

		return
	}

	http.Error(w, "all backends failed", 502)
}

func main() {

	flag.IntVar(&timeout, "timeout", 2000, "timeout ms")
	flag.IntVar(&cacheTTL, "cache-ttl", 10, "cache ttl seconds")

	flag.Parse()

	cfg := loadConfig("config.yaml")

	if len(cfg.Servers) == 0 {
		log.Fatal("no servers defined in config.yaml")
	}

	backendList = cfg.Servers

	client := http.Client{
		Timeout: time.Duration(timeout) * time.Millisecond,
	}

	for _, backend := range backendList {
		url := fmt.Sprintf("http://%s/", backend)

		start := time.Now()

		resp, err := client.Get(url)

		if err != nil {
			log.Println("backend warmup error:", backend, err)
			continue
		}

		_, err = io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			log.Println("backend warmup read error:", backend, err)
			continue
		}

		latency := time.Since(start)
		setBackendLatency(backend, latency)

		log.Println("backend warmup success:", backend, latency)
	}

	http.HandleFunc("/", handler)

	addr := fmt.Sprintf(":%d", port)

	log.Println("main server running on", addr)
	log.Println("backends:", backendList)

	log.Fatal(http.ListenAndServe(addr, nil))
}
