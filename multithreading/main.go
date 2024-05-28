package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"time"
)

const (
	brasilApiUrl = "https://brasilapi.com.br/api/cep/v1/%s"
	viaCepUrl    = "http://viacep.com.br/ws/%s/json"
)

var (
	queryTimeout time.Duration
	cepToQuery   string
)

type (
	QueryResult struct {
		Provider    string
		ElapsedTime time.Duration
		Data        string
	}
)

func (q QueryResult) String() string {
	d, _ := json.Marshal(q)
	return string(d)
}

func queryCep(ctx context.Context, provider, url string, result chan QueryResult) {
	url = fmt.Sprintf(url, cepToQuery)

	r := QueryResult{Provider: provider}
	start := time.Now()
	defer func() {
		r.ElapsedTime = time.Since(start)
		log.Printf("Finished %s in %v", provider, r.ElapsedTime)
		result <- r
	}()

	log.Printf("Querying %s (%s)", provider, url)
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	req, e := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if e != nil {
		log.Printf("[%s] Failed to create request: %v", provider, e)
		return
	}

	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		log.Printf("[%s] Failed to request: %v", provider, e)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[%s] Failed to request: %v", provider, resp.Status)
		return
	}

	data, e := io.ReadAll(resp.Body)
	if e != nil {
		log.Printf("[%s] Failed to read response: %+v - %v", provider, req, e)
		return
	}

	r.Data = string(data)
}

func init() {
	// parse args
	flag.DurationVar(&queryTimeout, "query-timeout", time.Second, "Timeout querying for CEPs")
	flag.StringVar(&cepToQuery, "cep", "", "CEP to query")
	flag.Parse()

	rx, _ := regexp.Compile(`^\d{5}[-]?\d{3}$`)
	if !rx.MatchString(cepToQuery) {
		log.Fatalf("Invalid CEP: %s", cepToQuery)
	}
}

func main() {
	result := make(chan QueryResult, 2)

	log.Println("Starting...")

	go queryCep(context.Background(), "ViaCEP", viaCepUrl, result)
	go queryCep(context.Background(), "BrasilAPI", brasilApiUrl, result)

	r1 := <-result
	r2 := <-result

	var winner QueryResult
	if r1.ElapsedTime <= r2.ElapsedTime {
		winner = r1
	} else {
		winner = r2
	}

	if winner.Data != "" {
		log.Printf("Winner: >> %s <<\n", winner.Provider)
		log.Printf("Elapsed time: %v\n", winner.ElapsedTime)
		log.Printf("Response: %s\n", winner.Data)
	} else {
		log.Fatalf("No winners!")
	}
}
