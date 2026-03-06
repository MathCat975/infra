package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Servers map[string][]string `yaml:"servers"`
}

var (
	port     = 80
	timeout  int
	cacheTTL int
)

var servers map[string][]string

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

	if len(servers) == 0 {
		http.Error(w, "no backends configured", http.StatusInternalServerError)
		return
	}

	if r.URL.Path == "/" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		keys := make([]string, 0, len(servers))
		for name := range servers {
			keys = append(keys, name)
		}
		sort.Strings(keys)

		fmt.Fprintln(w, "<html><head><title>Applications</title></head><body>")
		fmt.Fprintln(w, "<h1>Applications</h1>")
		fmt.Fprintln(w, "<ul>")
		for _, name := range keys {
			fmt.Fprintf(w, `<li><a href="/%s/">%s</a></li>`+"\n", name, name)
		}
		fmt.Fprintln(w, "</ul>")
		fmt.Fprintln(w, "</body></html>")
		return
	}

	client := http.Client{
		Timeout: time.Duration(timeout) * time.Millisecond,
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(path, "/", 2)
	appName := parts[0]

	if appName == "" {
		http.NotFound(w, r)
		return
	}

	backends, ok := servers[appName]
	if !ok || len(backends) == 0 {
		http.NotFound(w, r)
		return
	}

	restPath := "/"
	if len(parts) == 2 && parts[1] != "" {
		restPath += parts[1]
	}

	var bodyBytes []byte
	if r.Body != nil {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}
		bodyBytes = b
	}

	backendsCopy := make([]string, len(backends))
	copy(backendsCopy, backends)

	sort.Slice(backendsCopy, func(i, j int) bool {
		li, oki := getBackendLatency(backendsCopy[i])
		lj, okj := getBackendLatency(backendsCopy[j])

		if !oki {
			li = time.Hour
		}

		if !okj {
			lj = time.Hour
		}

		return li < lj
	})

	for i := 0; i < len(backendsCopy); i++ {

		backend := backendsCopy[i]

		u := &url.URL{
			Scheme:   "http",
			Host:     backend,
			Path:     restPath,
			RawQuery: r.URL.RawQuery,
		}

		start := time.Now()

		req, err := http.NewRequest(r.Method, u.String(), bytes.NewReader(bodyBytes))
		if err != nil {
			log.Println("request creation error:", backend, err)
			continue
		}
		req.Header = r.Header.Clone()

		resp, err := client.Do(req)

		if err != nil {
			log.Println("backend error:", backend, err)
			continue
		}

		latency := time.Since(start)
		setBackendLatency(backend, latency)

		for k, values := range resp.Header {
			for _, v := range values {
				w.Header().Add(k, v)
			}
		}

		w.WriteHeader(resp.StatusCode)

		_, err = io.Copy(w, resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Println("response write error:", backend, err)
		}

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

	servers = cfg.Servers

	client := http.Client{
		Timeout: time.Duration(timeout) * time.Millisecond,
	}

	for _, backendList := range servers {
		for _, backend := range backendList {
			u := fmt.Sprintf("http://%s/", backend)

			start := time.Now()

			resp, err := client.Get(u)

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
	}

	http.HandleFunc("/", handler)

	addr := fmt.Sprintf(":%d", port)

	log.Println("main server running on", addr)
	log.Println("servers:", servers)

	log.Fatal(http.ListenAndServe(addr, nil))
}
