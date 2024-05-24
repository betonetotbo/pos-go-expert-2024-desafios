package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/valyala/fastjson"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*300)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8080/cotacao", nil)
	if e != nil {
		panic(e)
	}

	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		panic(e)
	}

	body := resp.Body
	defer body.Close()

	data, e := io.ReadAll(body)
	if e != nil {
		panic(e)
	}

	log.Printf("%s\b", data)

	cotacao, e := fastjson.ParseBytes(data)
	if e != nil {
		panic(e)
	}

	bid := string(cotacao.GetStringBytes("bid"))
	log.Printf("Cotação: %v\n", bid)

	f, e := os.OpenFile("cotacao.txt", os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
	if e != nil {
		panic(e)
	}
	defer f.Close()

	fmt.Fprintf(f, "Dólar: %v\n", bid)
}
