package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

func main () {
	var apiCfg apiConfig

	mux := http.NewServeMux()
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	mux.HandleFunc("GET /healthz", handlerHealtz)
	mux.HandleFunc("GET /metrics", apiCfg.handlerGetNumOfReq)
	mux.HandleFunc("POST /reset", apiCfg.handlerCfgReset)

	server := http.Server{Handler: mux, Addr: ":8080"}

	err := server.ListenAndServe()
	if err != nil {
		fmt.Println(err)
	}
}

type apiConfig struct {
	fileserverHits atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w,r)
	})
}

func (cfg *apiConfig) handlerGetNumOfReq(w http.ResponseWriter, req *http.Request) {
	theString := fmt.Sprintf("Hits: %d", cfg.fileserverHits.Load())
	w.Write([]byte(theString))
}

func (cfg *apiConfig) handlerCfgReset(w http.ResponseWriter, req *http.Request) {
	cfg.fileserverHits.Store(0)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset to 0"))
}

func handlerHealtz(w http.ResponseWriter, req *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}




