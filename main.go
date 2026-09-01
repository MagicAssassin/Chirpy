package main

import (
	"chirpy/internal/auth"
	"chirpy/internal/database"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/alexedwards/argon2id"
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

	secret := os.Getenv("SECRET")
	apiCfg.secret = secret

	apiCfg.db = dbQueries
	apiCfg.platform = os.Getenv("PLATFORM")
	apiCfg.polkaKey = os.Getenv("POLKA_KEY")

	mux := http.NewServeMux()
	mux.Handle("/app/", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	
	mux.HandleFunc("GET /admin/metrics", apiCfg.handlerGetNumOfReq)
	mux.HandleFunc("POST /admin/reset", apiCfg.handlerCfgReset)

	mux.HandleFunc("POST /api/chirps", apiCfg.handlerValidateChirp)
	mux.HandleFunc("POST /api/users", apiCfg.handlerCreateUser)
	mux.HandleFunc("POST /api/login", apiCfg.handlerLogin)
	mux.HandleFunc("POST /api/refresh", apiCfg.handlerRefresh)
	mux.HandleFunc("POST /api/revoke", apiCfg.handlerRevoke)
	mux.HandleFunc("POST /api/polka/webhooks", apiCfg.handlerUpgradeToRed)

	mux.HandleFunc("GET /api/chirps", apiCfg.handlerGetAllChirps)
	mux.HandleFunc("GET /api/chirps/{chirpID}", apiCfg.handlerGetChirp)
	mux.HandleFunc("GET /api/healthz", handlerHealtz)
	
	mux.HandleFunc("PUT /api/users", apiCfg.handlerUpdateEmailAndPass)

	mux.HandleFunc("DELETE /api/chirps/{chirpID}", apiCfg.handlerDeleteChirp)

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
	secret string
	polkaKey string
}

func HashPassword(password string) (string, error) {
	hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return "", err
	}

	return hash, nil
}

func CheckPasswordHash(password, hash string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, err
	}

	return match, nil
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
		return
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

func (cfg *apiConfig) handlerValidateChirp(w http.ResponseWriter, req *http.Request) {
	type ValidateChirpParams struct {
        Body string `json:"body"`
    }

	type returnVals struct {
        ID uuid.UUID `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Body string `json:"body"`
		UserID uuid.UUID `json:"user_id"`
    }

	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		respondWithError(w, 401, "Something went wrong")
		return
	}

	UserID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, 401, "Something went wrong")
		return
	}

	decoder := json.NewDecoder(req.Body)
    params := ValidateChirpParams{}
    err = decoder.Decode(&params)
    if err != nil {
		respondWithError(w, 400, "Something went wrong")
		return
    }

	if len(params.Body) > 140 {
		respondWithError(w, 400, "Chirp is too long")
		return
	}

	CBody := cleanBody(params.Body)

	nullUUID := uuid.NullUUID{
		UUID:  UserID,
		Valid: true,
	}

	chirpsParams := database.CreateChirpParams{Body: CBody, UserID: nullUUID}
	chirp, err := cfg.db.CreateChirp(req.Context(), chirpsParams)
	if err != nil {
		respondWithError(w, 400, "Failed to Create Chirp")
		return
	}

	respBody := returnVals{
        ID: chirp.ID,
		CreatedAt: chirp.CreatedAt.String(),
		UpdatedAt: chirp.UpdatedAt.String(),
		Body: chirp.Body,
		UserID: chirp.UserID.UUID,
    }

	respondWithJSON(w, 201, respBody)

}

func (cfg *apiConfig) handlerGetAllChirps(w http.ResponseWriter, req *http.Request) {
	type returnVals struct {
        ID uuid.UUID `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Body string `json:"body"`
		UserID uuid.UUID `json:"user_id"`
    }

	s := req.URL.Query().Get("author_id")

	var userID uuid.UUID
	var err error

	if s != "" {
		userID, err = uuid.Parse(s)
		if err != nil {
			respondWithError(w, 400, "Invalid User ID format")
			return
		}
	} else {
		userID = uuid.Nil
	}

	sortValue := req.URL.Query().Get("sort")

	switch sortValue {
		case "asc":
			sortValue = "ASC"
		case "desc":
			sortValue = "DESC"
		default:
			sortValue = "ASC"
    }

	getChirpsParams := database.GetChirpsParams{Column1: userID, Column2: sortValue}

	chirpsList, err := cfg.db.GetChirps(req.Context(), getChirpsParams)
	if err != nil {
		respondWithError(w, 400, "Trouble Getting Chirps")
		return
	}

	var returnList []returnVals

	for _, chirp := range chirpsList {
		returnList = append(returnList, returnVals{ID: chirp.ID, CreatedAt: chirp.CreatedAt.String(), UpdatedAt: chirp.UpdatedAt.String(), Body: chirp.Body, UserID: chirp.UserID.UUID})
	}

	respondWithJSON(w, 200, returnList)
}

func (cfg *apiConfig) handlerGetChirp(w http.ResponseWriter, req *http.Request) {
	type returnVals struct {
        ID uuid.UUID `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Body string `json:"body"`
		UserID uuid.UUID `json:"user_id"`
    }

	parsedID, err := uuid.Parse(req.PathValue("chirpID"))
	if err != nil {
		respondWithError(w, 400, "Error Parsing ID")
		return
	}

	chirp, err := cfg.db.GetChirpByID(req.Context(), parsedID)
	if err != nil {
		respondWithError(w, 404, "Error Getting Chirp")
		return
	}

	theValue := returnVals{ID: chirp.ID, CreatedAt: chirp.CreatedAt.String(), UpdatedAt: chirp.UpdatedAt.String(), Body: chirp.Body, UserID: chirp.UserID.UUID}
	respondWithJSON(w, 200, theValue)
}

func (cfg *apiConfig) handlerCreateUser(w http.ResponseWriter, req *http.Request) {
	type userInfoParams struct {
		Email string `json:"email"`
		Password string `json:"password"`
	}
	type userReturnValues struct {
		Id uuid.UUID `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Email string `json:"email"`
		IsChirpRed bool `json:"is_chirpy_red"`
	}

	decoder := json.NewDecoder(req.Body)
    params := userInfoParams{}
    err := decoder.Decode(&params)
    if err != nil {
		respondWithError(w, 400, "Something went wrong")
		return
    }

	ctx := req.Context()

	hashPassword, err := auth.HashPassword(params.Password)
	if err != nil {
		respondWithError(w, 400, "Problem Hassing Password")
	}

	userParams := database.CreateUserParams{Email: params.Email, HashedPassword: hashPassword}

	user, err := cfg.db.CreateUser(ctx, userParams)
	if err != nil {
		respondWithError(w, 400, "Error Creating User")
		return
	}

	returnValues := userReturnValues{Id: user.ID, CreatedAt: user.CreatedAt.String(), UpdatedAt: user.UpdatedAt.String(), Email: user.Email, IsChirpRed: user.IsChirpyRed}
	respondWithJSON(w, 201, returnValues)

	
}

func (cfg *apiConfig) handlerLogin(w http.ResponseWriter, req *http.Request) {
	type userInfoParams struct {
		Email string `json:"email"`
		Password string `json:"password"`
		ExipireIn int `json:"expires_in_seconds"`
	}
	type userReturnValues struct {
		Id uuid.UUID `json:"id"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
		Email string `json:"email"`
		Refresh_Token string `json:"refresh_token"`
		Token string `json:"token"`
		IsChirpRed bool `json:"is_chirpy_red"`
		
	}

	decoder := json.NewDecoder(req.Body)
    params := userInfoParams{}
    err := decoder.Decode(&params)
    if err != nil {
		respondWithError(w, 400, "Something went wrong")
		return
    }

	user, err := cfg.db.GetUserByEmail(req.Context(), params.Email)
	if err != nil {
		respondWithError(w, 401, "Incorrect email or password")
		return
    }

	match, err := auth.CheckPasswordHash(params.Password, user.HashedPassword)
	if err != nil {
		respondWithError(w, 401, "Incorrect email or password")
		return
    }

	if match == false {
		respondWithError(w, 401, "Incorrect email or password")
		return
	}

	token, err := auth.MakeJWT(user.ID, cfg.secret, time.Duration(3600) * time.Second)
	if err != nil {
		respondWithError(w, 400, "Trouble Logging In")
		return
	}

	nullUUID := uuid.NullUUID{
		UUID:  user.ID,
		Valid: true, 
	}

	refreshTokenParams := database.CreateRefreshTokenParams{Token: auth.MakeRefreshToken(), ExpiresAt: time.Now().Add(time.Duration(86400) * time.Minute), UserID:  nullUUID}
	refreshToken, err := cfg.db.CreateRefreshToken(req.Context(), refreshTokenParams)
	if err != nil {
		respondWithError(w, 401, "Trouble Logging In")
		return
	}

	returnValues := userReturnValues{Id: user.ID, CreatedAt: user.CreatedAt.String(), UpdatedAt: user.UpdatedAt.String(), Email: user.Email, Token: token, Refresh_Token: refreshToken.Token, IsChirpRed: user.IsChirpyRed}
	respondWithJSON(w, 200, returnValues)
}

func (cfg *apiConfig) handlerRefresh (w http.ResponseWriter, req *http.Request) {
	type returnValues struct {
		Token string `json:"token"`
	}

	reqToken, err := auth.GetBearerToken(req.Header)
	if err != nil {
		respondWithError(w, 400, "There was some problem")
		return
	}

	refreshToken, err := cfg.db.GetRefreshTokenByTok(req.Context(), reqToken)
	if err != nil {
		respondWithError(w, 401, "Trouble Refreshing")
		return
	}

	if time.Now().After(refreshToken.ExpiresAt) {
		respondWithError(w, 401, "Trouble Refreshing")
		return
	}

	if refreshToken.RevokedAt.Valid == true {
		respondWithError(w, 401, "Trouble Refreshing")
		return
	}
	
	token, err := auth.MakeJWT(refreshToken.UserID.UUID, cfg.secret, time.Duration(3600) * time.Second)
	if err != nil {
		respondWithError(w, 401, "Trouble Refreshing")
		return
	}

	returedValues := returnValues{Token: token}
	respondWithJSON(w, 200, returedValues)
}

func (cfg *apiConfig) handlerRevoke (w http.ResponseWriter, req *http.Request) {
	type returnValues struct {
	}

	reqToken, err := auth.GetBearerToken(req.Header)
	if err != nil {
		respondWithError(w, 400, "There was some problem")
		return
	}

	err = cfg.db.UpdateRevokeAt(req.Context(), reqToken)
	if err != nil {
		respondWithError(w, 400, "There was some problem")
		return
	}

	respondWithJSON(w, 204, returnValues{})
}

func (cfg *apiConfig) handlerUpdateEmailAndPass (w http.ResponseWriter, req *http.Request) {
	type updateInfoParams struct {
		Email string `json:"email"`
		Password string `json:"password"`
	}
	type returnValues struct {
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(req.Body)
    params := updateInfoParams{}
    err := decoder.Decode(&params)
    if err != nil {
		respondWithError(w, 401, "Something went wrong")
		return
    }

	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		respondWithError(w, 401, "There was some problem")
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, 401, "Unauthorised Access")
		return
	}

	HPassword, err := auth.HashPassword(params.Password)

	updateDBParams := database.UpdateUserEmailPassParams{Email: params.Email, HashedPassword: HPassword, ID: userID}

	user, err := cfg.db.UpdateUserEmailPass(req.Context(), updateDBParams)
	if err != nil {
		respondWithError(w, 401, "Unauthorised Access")
		return
	}

	returedValues := returnValues{Email: user.Email}
	respondWithJSON(w, 200, returedValues)

}

func (cfg *apiConfig) handlerDeleteChirp (w http.ResponseWriter, req *http.Request) {
	type returnValues struct {
	}

	token, err := auth.GetBearerToken(req.Header)
	if err != nil {
		respondWithError(w, 401, "Unauthorised Access")
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, 401, "Unauthorised Access")
		return
	}

	nullUUID := uuid.NullUUID{
		UUID:  userID,
		Valid: true, 
	}

	parsedUUID, err := uuid.Parse(req.PathValue("chirpID"))
	if err != nil {
		respondWithError(w, 403, "Unauthorised Access3")
		return
	}

	chirp, err := cfg.db.GetChirpByID(req.Context(), parsedUUID)
	if err != nil {
		respondWithError(w, 404, "Chirp Does Not Exist")
		return
	}
	
	if chirp.UserID.UUID != userID {
		respondWithError(w, 403, "Unauthorised Access1")
		return
	}

	deleteChirpDBParams := database.DeleteChirpParams{UserID: nullUUID, ID: chirp.ID}
	err = cfg.db.DeleteChirp(req.Context(), deleteChirpDBParams)
	if err != nil {
		respondWithError(w, 400, "Somthing Went Wrong")
		return
	}

	respondWithJSON(w, 204, returnValues{})
}

func (cfg *apiConfig) handlerUpgradeToRed (w http.ResponseWriter, req *http.Request) {
	type insideData struct {
		UserId uuid.UUID `json:"user_id"`
	}
	type upgradeInfoParams struct {
		Event string `json:"event"`
		Data insideData `json:"data"`
	}
	type emptyReturnValues struct {
	}

	apiKey, err := auth.GetAPIKey(req.Header)
	if err != nil {
		respondWithError(w, 401, "Could not get api key")
		return
	}
	
	if apiKey != cfg.polkaKey {
		respondWithError(w, 401, "Something went wrong")
		return
	}
	
	decoder := json.NewDecoder(req.Body)
    params := upgradeInfoParams{}
    err = decoder.Decode(&params)
    if err != nil {
		respondWithError(w, 401, "Something went wrong")
		return
    }

	if params.Event == "user.upgraded" {
		respondWithJSON(w, 204, emptyReturnValues{})
	}

	err = cfg.db.UpgradeChirpyRedByID(req.Context(), params.Data.UserId)
	if err != nil {
		respondWithError(w, 404, "User Can't Be found")
		return
	}

	respondWithJSON(w, 204, emptyReturnValues{})
}