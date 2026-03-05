
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"
)

type Config struct {
	port        int
	delayMin    int
	delayMax    int
	errorRate   float64
	garbageRate float64
}

var cfg Config

func main() {

	flag.IntVar(&cfg.port, "port", 8080, "port du serveur")
	flag.IntVar(&cfg.delayMin, "delay-min", 50, "delay minimum en ms")
	flag.IntVar(&cfg.delayMax, "delay-max", 500, "delay maximum en ms")
	flag.Float64Var(&cfg.errorRate, "error-rate", 0.1, "probabilité d'erreur HTTP")
	flag.Float64Var(&cfg.garbageRate, "garbage-rate", 0.2, "probabilité de réponse pourrie")

	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/page", handlePage)
	http.HandleFunc("/api/data", handleAPI)
	http.HandleFunc("/healthz", handleHealth)

	addr := fmt.Sprintf(":%d", cfg.port)

	log.Println("Serveur secondaire lancé sur", addr)
	log.Printf("delay=%d-%d ms errorRate=%.2f garbageRate=%.2f\n",
		cfg.delayMin, cfg.delayMax, cfg.errorRate, cfg.garbageRate)

	log.Fatal(http.ListenAndServe(addr, nil))
}

func randomDelay() {
	delay := rand.Intn(cfg.delayMax-cfg.delayMin+1) + cfg.delayMin
	time.Sleep(time.Duration(delay) * time.Millisecond)
}

func maybeError(w http.ResponseWriter) bool {

	if rand.Float64() < cfg.errorRate {

		status := []int{500, 502, 503, 404}[rand.Intn(4)]

		http.Error(w, "simulated error", status)

		return true
	}

	return false
}

func handleRoot(w http.ResponseWriter, r *http.Request) {

	randomDelay()

	if maybeError(w) {
		return
	}

	fmt.Fprintf(w, `
<html>
<body>
<h1>Secondary server</h1>
<ul>
<li><a href="/page?id=1">page</a></li>
<li><a href="/api/data?key=test">api</a></li>
<li><a href="/healthz">health</a></li>
</ul>
</body>
</html>
`)
}

func handlePage(w http.ResponseWriter, r *http.Request) {

	randomDelay()

	if maybeError(w) {
		return
	}

	id := r.URL.Query().Get("id")

	if rand.Float64() < cfg.garbageRate {

		fmt.Fprintf(w, "<html><body><h1>PAGE %s<p>broken html...", id)

		return
	}

	fmt.Fprintf(w, `
<html>
<body>
<h1>Page %s</h1>
<p>random value: %d</p>
</body>
</html>
`, id, rand.Intn(10000))
}

func handleAPI(w http.ResponseWriter, r *http.Request) {

	randomDelay()

	if maybeError(w) {
		return
	}

	key := r.URL.Query().Get("key")

	if rand.Float64() < cfg.garbageRate {

		w.Header().Set("Content-Type", "application/json")

		fmt.Fprintf(w, `{"key":%s,"broken":true`, key)

		return
	}

	resp := map[string]interface{}{
		"key":   key,
		"value": rand.Intn(1000),
		"time":  time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")

	json.NewEncoder(w).Encode(resp)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {

	w.WriteHeader(200)

	fmt.Fprintln(w, "ok")
}