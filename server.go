package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"os"

	"github.com/garyburd/redigo/redis"
)

//  Globals
var (
	Pool              *redis.Pool
	QUOTE_SERVER_ADDR = os.Getenv("LEGACY_QUOTE_SERVER_ADDR")
	QUOTE_SERVER_PORT = os.Getenv("LEGACY_QUOTE_SERVER_PORT")
	config            = quoteConfig{func() string {
		if runningInDocker() {
			return "redis"
		} else {
			return "localhost"
		}
	}(), QUOTE_SERVER_ADDR + QUOTE_SERVER_PORT}
)

type Quote struct {
	Price       string
	StockSymbol string
	UserId      string
	Timestamp   int64
	CryptoKey   string
	Cached      bool
}

type quoteConfig struct {
	redis       string
	quoteServer string
}

func runningInDocker() bool {
	_, err := os.Stat("/.dockerenv")
	if err == nil {
		return true
	}
	return false
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

	c := Pool.Get()
	defer c.Close()

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
			//fmt.Println("cache GET", q)
			q.Cached = true
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
		//fmt.Println("not in cache", err)
		conn, err := net.Dial("tcp", config.quoteServer)
		if err != nil {
			fmt.Println("Couldnt connect to: ", config.quoteServer)
			fmt.Println("Connection error", err)
			return Quote{}, err
		}

		defer conn.Close()

		commandString := stockSymbol + "," + userId

		conn.Write([]byte(commandString + "\n"))
		buff := make([]byte, 1024)
		length, _ := conn.Read(buff)
		quoteString := string(buff[:length])

		if quoteString == "" {
			failGracefully(err, "quote was empty")
		}
		// Parse Quote
		quoteStringComponents := strings.Split(quoteString, ",")
		thisQuote := Quote{}

		thisQuote.Price = quoteStringComponents[0]
		thisQuote.StockSymbol = quoteStringComponents[1]
		thisQuote.UserId = userId
		thisQuote.Timestamp, _ = strconv.ParseInt(quoteStringComponents[3], 10, 64)
		thisQuote.CryptoKey = quoteStringComponents[4]
		thisQuote.Cached = false

		currentQuoteJSON, err := json.Marshal(thisQuote)

		if err == nil {
			//successful legacy get
			//fmt.Println("LEGACY GET", string(currentQuoteJSON))
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

	//w.WriteHeader(http.StatusOK)
	w.Write(currentQuoteJSON)
}

// func loadDB() redis.Conn {
// 	var c redis.Conn
// 	var rederr error
// 	for i := 0; i < 5; i++ {
// 		time.Sleep(time.Duration(i) * time.Second)

// 		c, rederr = redis.Dial("tcp", config.redis+":6379")
// 		if rederr != nil {
// 			fmt.Println("Could not connect to Redis:", rederr)
// 		}

// 		if rederr == nil {
// 			break
// 		}
// 		log.Println(rederr)
// 	}

// 	if rederr != nil {
// 		failGracefully(rederr, "Failed to open Redis")
// 	} else {
// 		fmt.Println("Connected to Redis")
// 	}

// 	return c
// }

func initDB() {
	redisHost := config.redis + ":6379"
	Pool = newPool(redisHost)
	cleanupHook()
}

func cleanupHook() {

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	signal.Notify(c, syscall.SIGKILL)
	go func() {
		<-c
		Pool.Close()
		os.Exit(0)
	}()
}

func newPool(server string) *redis.Pool {

	return &redis.Pool{

		MaxIdle:     80,
		MaxActive:   10000,
		IdleTimeout: 30 * time.Second,

		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server)
			if err != nil {
				return nil, err
			}
			return c, err
		},

		// TestOnBorrow: func(c redis.Conn, t time.Time) error {
		// 	_, err := c.Do("PING")
		// 	return err
		// },
	}
}

func main() {

	initDB()
	port := ":44418"
	fmt.Printf("Listening on port %s\n", port)
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/quote", quoteHandler)
	http.ListenAndServe(port, nil)

}
