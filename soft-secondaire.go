
package main

import (
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
	garbageRate float64
}

var cfg Config

func main() {

	cfg.port = 80
	flag.IntVar(&cfg.delayMin, "delay-min", 50, "delay minimum en ms")
	flag.IntVar(&cfg.delayMax, "delay-max", 500, "delay maximum en ms")

	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	http.HandleFunc("/", handlePage)

	addr := fmt.Sprintf(":%d", cfg.port)

	log.Println("Serveur secondaire lancé sur", addr)
	log.Printf("delay=%d-%d ms\n",
		cfg.delayMin, cfg.delayMax)

	log.Fatal(http.ListenAndServe(addr, nil))
}

func randomDelay() {
	delay := rand.Intn(cfg.delayMax-cfg.delayMin+1) + cfg.delayMin
	time.Sleep(time.Duration(delay) * time.Millisecond)
}


func handlePage(w http.ResponseWriter, r *http.Request) {

	randomDelay()

	id := r.URL.Query().Get("id")

	fmt.Fprintf(w, `
<html>
<body>
<h1>Page %s</h1>
<p>random value: %d</p>
</body>
</html>
`, id, rand.Intn(10000))
}
