package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/mattn/go-sqlite3"
)

type (
	ErrorHandlerFunc = func(w http.ResponseWriter, r *http.Request) error

	ExchangeResult struct {
		Code       string  `json:"code"`
		Codein     string  `json:"codein"`
		Name       string  `json:"name"`
		High       float64 `json:"high,string"`
		Low        float64 `json:"low,string"`
		VarBid     float64 `json:"varBid,string"`
		PctChange  float64 `json:"pctChange,string"`
		Bid        float64 `json:"bid,string"`
		Ask        float64 `json:"ask,string"`
		Timestamp  int64   `json:"timestamp,string"`
		CreateDate string  `json:"create_date"`
	}

	UsdbrlResult struct {
		ExchangeResult `json:"USDBRL"`
	}

	HttpError struct {
		Message string `json:"message"`
	}
)

var (
	port           int
	queryTimeout   time.Duration
	persistTimeout time.Duration
	db             *sql.DB
)

const (
	exchagneRateUrl = "https://economia.awesomeapi.com.br/json/last/USD-BRL"
)

func init() {
	// parse args
	flag.IntVar(&port, "port", 8080, "HTTP server port")
	flag.DurationVar(&queryTimeout, "query-timeout", time.Millisecond*200, "Time to request for exchange rate")
	flag.DurationVar(&persistTimeout, "persist-timeout", time.Millisecond*10, "Time to persist exchange rate")
	flag.Parse()

	// open database
	var e error
	db, e = sql.Open("sqlite3", "file:cotacoes.sqlite")
	if e != nil {
		log.Fatalf("Failed to open database: %v\n", e)
	}
	// prepare migrations
	drv, e := sqlite3.WithInstance(db, &sqlite3.Config{})
	if e != nil {
		log.Fatalf("Failed to open database: %v\n", e)
	}
	// configure migrations source
	m, e := migrate.NewWithDatabaseInstance("file://./migrations/", "sqlite3", drv)
	if e != nil {
		log.Fatalf("Failed to load database migrations: %v\n", e)
	}
	// migrate!
	e = m.Up()
	if e != nil && e.Error() != "no change" {
		log.Fatalf("Failed to run migrations: %v\n", e)
	}
}

// Insert exchange rate into database
func insertExchangeRate(ctx context.Context, e *ExchangeResult) error {
	ctx, cancel := context.WithTimeout(ctx, persistTimeout)
	defer cancel()

	query := `INSERT INTO exchanges (code, codein, name, high, low, varbid, pctchange, bid, ask, timestamp, createdate) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := db.ExecContext(ctx, query, e.Code, e.Codein, e.Name, e.High, e.Low, e.VarBid, e.PctChange, e.Bid, e.Ask, e.Timestamp, e.CreateDate)
	return err
}

// Request exchange rate to external API
func requestExchangeRate(ctx context.Context, r *UsdbrlResult) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	req, e := http.NewRequestWithContext(ctx, http.MethodGet, exchagneRateUrl, nil)
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")

	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return e
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Unexpected status code: %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(r)
}

// Exchange rate endpoint
func exchangeRate(w http.ResponseWriter, r *http.Request) error {
	var result UsdbrlResult
	e := requestExchangeRate(r.Context(), &result)
	if e != nil {
		return e
	}

	e = insertExchangeRate(r.Context(), &result.ExchangeResult)
	if e != nil {
		return e
	}

	e = json.NewEncoder(w).Encode(result.ExchangeResult)
	if e != nil {
		log.Printf("Failed to encode response: %v\n", e)
	}

	return nil
}

// Middleware to capture errors and write them as HTTP response
func errorMiddleware(h ErrorHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		e := h(w, r)
		if e != nil {
			debug.PrintStack()
			log.Printf("An error has occurred: %v\n", e)
			w.Header().Add("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			he := &HttpError{Message: e.Error()}
			json.NewEncoder(w).Encode(he)
		}
	}
}

// register the endpoints
func createMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /cotacao", errorMiddleware(exchangeRate))

	return mux
}

func main() {
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: createMux(),
	}

	go func() {
		log.Printf("Server listening on port %d\n", port)
		e := server.ListenAndServe()
		if e != nil && e != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v\n", e)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	log.Println("Waiting for signal...")
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	log.Println("Shuting down server...")
	e := server.Shutdown(ctx)
	if e != nil {
		log.Fatalf("Failed to graceful shutdown: %v\n", e)
	}

	log.Println("Server stopped")
}
