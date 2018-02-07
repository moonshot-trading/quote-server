package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
)

//  Globals
var (
	c = loadDB()
)

type Quote struct {
	Price       float64
	StockSymbol string
	UserId      string
	Timestamp   int64
	CryptoKey   string
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

	q := Quote{}
	if c == nil {
		fmt.Println("lol no db haha")
	}
	v, err := redis.String(c.Do("GET", stockSymbol))
	//	scanerr := redis.ScanStruct(v, &q)
	if err == nil {
		b := ([]byte)(v)

		if scanerr := json.Unmarshal(b, &q); scanerr != nil {
			return Quote{}, err
		} else {
			fmt.Println("cache GET", q)
			return q, err
		}
	} else {

		// if err == nil && scanerr != nil {
		// 	fmt.Println("SCAN ERROR")
		// 	return q, scanerr
		// }
		// if err == nil && scanerr == nil {
		// 	fmt.Println("HEATHER GET", q)
		// 	return q, nil
		// } else {
		//  No cached quote, go get a new quote
		fmt.Println("not in cache", err)
		conn, err := net.Dial("tcp", "docker.for.mac.localhost:44415")
		if err != nil {
			fmt.Println("Connection error", err)
			return Quote{}, err
		}

		defer conn.Close()

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

		currentQuoteJSON, err := json.Marshal(thisQuote)

		if err == nil {
			//successful legacy get
			fmt.Println("LEGACY GET", string(currentQuoteJSON))
			c.Send("MULTI")
			c.Send("SET", stockSymbol, string(currentQuoteJSON))
			c.Send("EXPIRE", stockSymbol, "60")
			_, erro := c.Do("EXEC")

			if erro != nil {
				//couldnt set to redis
				fmt.Println("COULDNT SET REDIS")
				return thisQuote, erro
			}
		}

		if err != nil {
			//unsuccessful legacy marshal
			return thisQuote, err
		}
		//  Return the quote
		return thisQuote, nil
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func quoteHandler(w http.ResponseWriter, r *http.Request) {

	decoder := json.NewDecoder(r.Body)
	req := struct {
		UserId      string
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

func loadDB() redis.Conn {
	var c redis.Conn
	var rederr error
	for i := 0; i < 5; i++ {
		time.Sleep(time.Duration(i) * time.Second)

		c, rederr = redis.Dial("tcp", "redis:6379")
		fmt.Println(rederr, "AHAHAHAHA")
		if rederr != nil {
			fmt.Println("Could not connect:", rederr)
		}

		if rederr == nil {
			break
		}
		log.Println(rederr)
	}

	if rederr != nil {
		failGracefully(rederr, "Failed to open Redis")
	} else {
		fmt.Println("Connected to DB")
	}

	return c
}

func main() {

	port := ":44418"
	fmt.Printf("Listening on port %s\n", port)
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/quote", quoteHandler)
	http.ListenAndServe(port, nil)

}
