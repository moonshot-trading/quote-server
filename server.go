package main

import (
    "fmt"
	"net/http"
	"net"
    "time"
	"encoding/json"
	"strings"
	"strconv"
)

//  Globals
var (
    quoteMap = make(map[string]Quote)
)

type Quote struct {
    Price float64
    StockSymbol string
    UserId string
    Timestamp int64
    CryptoKey string
}

func failWithStatusCode(err error, msg string, w http.ResponseWriter, statusCode int) {
    failGracefully(err, msg)
    w.WriteHeader(statusCode)
    fmt.Fprintf(w, msg)
}

func failGracefully(err error, msg string) {
    if err != nil {
        fmt.Printf("%s: %s", msg, err)
    }
}

func getQuote(userId string, stockSymbol string) (Quote, error) {
	quoteTime := int64(time.Now().Unix())

    //  Check if theres a cache entry for this
    if cachedQuote, exists := quoteMap[stockSymbol]; exists {
        //  Check if its valid, then return it
        if cachedQuote.Timestamp + 60 > quoteTime {
			//  Quote is good
            return cachedQuote, nil
        }
    }

    //  No cached quote, go get a new quote
    conn, err := net.Dial("tcp", "localhost:44415")
    defer conn.Close()

    if err != nil {
        fmt.Println("Connection error")
        return Quote{}, err
    }

    commandString := userId + "," + stockSymbol

    conn.Write([]byte(commandString + "\n"))
    buff := make([]byte, 1024)
    length, _ := conn.Read(buff)
    quoteString := string(buff[:length])

    // Parse Quote
    quoteStringComponents := strings.Split(quoteString, ",")
    thisQuote := Quote{}

    thisQuote.Price, _ = strconv.ParseFloat(quoteStringComponents[0], 64)
    thisQuote.StockSymbol = quoteStringComponents[1]
    thisQuote.UserId = userId
    thisQuote.Timestamp, _ = strconv.ParseInt(quoteStringComponents[3], 10, 64)
	thisQuote.CryptoKey = quoteStringComponents[4]
	
	quoteMap[stockSymbol] = thisQuote

    //  Log to the audit server

    //  Return the quote
    return thisQuote, nil
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
}

func quoteHandler(w http.ResponseWriter, r *http.Request) {
    decoder := json.NewDecoder(r.Body)
    req := struct {
        UserId string
        StockSymbol string
    }{"", ""}

    err := decoder.Decode(&req)

    if err != nil || len(req.StockSymbol) < 3 {
        failWithStatusCode(err, http.StatusText(http.StatusBadRequest), w, http.StatusBadRequest)
        return
    }

    //  Get quote here
	currentQuote, err := getQuote(req.UserId, req.StockSymbol)
	
	if err != nil {
		failWithStatusCode(err, http.StatusText(http.StatusInternalServerError), w, http.StatusInternalServerError)
		return
	}

    //  Return quote
	currentQuoteJSON, err := json.Marshal(currentQuote)
	
    w.WriteHeader(http.StatusOK)
    w.Write(currentQuoteJSON)
}

func main() {
    port := ":44418"
    fmt.Printf("Listening on port %s\n", port)
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/quote", quoteHandler)
    http.ListenAndServe(port, nil)
}