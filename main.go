package main

import (
	"chirpy/internal/database"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"github.com/google/uuid"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)



func main () {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	var apiCfg apiConfig

	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Error Opening Database")
	}
	dbQueries := database.New(db)

	apiCfg.db = dbQueries
	apiCfg.platform = os.Getenv("PLATFORM")

	mux := http.NewServeMux()
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	
	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerGetNumOfReq)
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerCfgReset)

	mux.HandleFunc("POST /api/validate_chirp", handlerValidateChirp)
	mux.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)
	mux.HandleFunc("GET /api/healthz", handlerHealtz)

	server := http.Server{Handler: mux, Addr: ":8080"}

	err = server.ListenAndServe()
	if err != nil {
		fmt.Println(err)
	}
}

type apiConfig struct {
	fileserverHits atomic.Int32
	db *database.Queries
	platform string
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHits.Add(1)
		next.ServeHTTP(w,r)
	})
}

func (cfg *apiConfig) handlerGetNumOfReq(w http.ResponseWriter, req *http.Request) {
	w.Header().Add("Content-Type", "text/html; charset=utf-8")

	theString := fmt.Sprintf("<html><body><h1>Welcome, Chirpy Admin</h1><p>Chirpy has been visited %d times!</p></body></html>", cfg.fileserverHits.Load())
	w.Write([]byte(theString))
}

func (cfg *apiConfig) handlerCfgReset(w http.ResponseWriter, req *http.Request) {
	if cfg.platform != "dev" {
		respondWithError(w, 403, "Forbidden")
	}
	
	cfg.fileserverHits.Store(0)

	err := cfg.db.DeleteAllUser(req.Context())
	if err != nil {
		respondWithError(w, 400, "Error Deleting Users")
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset to 0"))
}

func handlerHealtz(w http.ResponseWriter, req *http.Request) {
	w.Header().Add("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(200)
	w.Write([]byte("OK"))
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errReturnVal struct {
		Err string `json:"error"`
	}

	respError := errReturnVal{Err: msg}

	dat, err := json.Marshal(respError)
	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(400)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(dat)
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	dat, err := json.Marshal(payload)
	if err != nil {
		respondWithError(w, 400, "Something went wrong")
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
    w.Write(dat)
}

func cleanBody(s string) string {
	listOfWords := strings.Split(s, " ")
	for i, word := range listOfWords {
		if strings.ToLower(word) == "kerfuffle" || strings.ToLower(word) == "sharbert" || strings.ToLower(word) == "fornax" {
			listOfWords[i] = "****"
		}
	}

	result := strings.Join(listOfWords, " ")
	return result
}

func handlerValidateChirp(w http.ResponseWriter, req *http.Request) {
	type ValidateChirpParams struct {
        Body string `json:"body"`
    }

	type returnVals struct {
        CleanedBody string `json:"cleaned_body"`
    }

	decoder := json.NewDecoder(req.Body)
    params := ValidateChirpParams{}
    err := decoder.Decode(&params)
    if err != nil {
		respondWithError(w, 400, "Something went wrong")
		return
    }

	if len(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	CBody := cleanBody(params.Body)

	respBody := returnVals{
        CleanedBody: CBody,
    }

	respondWithJSON(w, 200, respBody)

}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, req *http.Request) {
	type userInfoParams struct {
		Email string `json:"email"`
	}
	type userReturnValues struct {
		Id uuid.UUID `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(req.Body)
    params := userInfoParams{}
    err := decoder.Decode(&params)
    if err != nil {
		respondWithError(w, 400, "Something went wrong")
		return
    }

	ctx := req.Context()

	user, err := cfg.db.CreateUser(ctx, params.Email)
	if err != nil {
		respondWithError(w, 400, "Error Creating User")
		return
	}

	returnValues := userReturnValues{Id: user.ID, CreatedAt: user.CreatedAt.String(), UpdatedAt: user.UpdatedAt.String(), Email: user.Email}
	respondWithJSON(w, 201, returnValues)

	
}


