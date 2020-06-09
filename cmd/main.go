package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/plaid/plaid-go/plaid"
)

type Linker struct {
	Results chan string
	Errors  chan error
	Client  *plaid.Client
}

func (l *Linker) Link(publicToken string) (plaid.ExchangePublicTokenResponse, error) {
	return l.Client.ExchangePublicToken(publicToken)
}

func main() {
	opts := plaid.ClientOptions{
		os.Getenv("PLAID_CLIENT_ID"),
		os.Getenv("PLAID_SECRET"),
		os.Getenv("PLAID_PUBLIC_KEY"),
		plaid.Development,
		&http.Client{},
	}

	client, err := plaid.NewClient(opts)

	if err != nil {
		log.Fatal(err)
	}

	linker := &Linker{
		Results: make(chan string),
		Errors:  make(chan error),
		Client:  client,
	}

	fmt.Println("Now Listening on 8000")
	go func() {
		http.HandleFunc("/link", handleLink(linker))
		log.Fatal(http.ListenAndServe(":8000", nil))
	}()

	select {
	case err := <-linker.Errors:
		log.Fatal(err)
	case publicToken := <-linker.Results:
		res, err := linker.Link(publicToken)
		if err != nil {
			log.Fatal(err)
		}

		log.Print(res)
	}
}

func handleLink(linker *Linker) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			http.ServeFile(w, r, "./static/link.html")
		case http.MethodPost:
			r.ParseForm()
			token := r.Form.Get("public_token")
			if token != "" {
				linker.Results <- token
			} else {
				linker.Errors <- errors.New("Empty public_token")
			}

		default:
			linker.Errors <- errors.New("Invalid HTTP method")
		}
	}
}
