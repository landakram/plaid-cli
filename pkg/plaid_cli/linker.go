package plaid_cli

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/plaid/plaid-go/plaid"
	"github.com/skratchdot/open-golang/open"
)

type Linker struct {
	Results chan string
	Errors  chan error
	Client  *plaid.Client
}

type TokenPair struct {
	ItemID      string
	AccessToken string
}

func (l *Linker) Link(port string) (*TokenPair, error) {
	fmt.Println(fmt.Sprintf("Starting Plaid Link on port %s", port))
	go func() {
		http.HandleFunc("/link", handleLink(l))
		err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
		if err != nil {
			l.Errors <- err
		}
	}()

	open.Run(fmt.Sprintf("http://localhost:%s/link", port))

	select {
	case err := <-l.Errors:
		return nil, err
	case publicToken := <-l.Results:
		res, err := l.exchange(publicToken)
		if err != nil {
			return nil, err
		}

		pair := &TokenPair{
			ItemID:      res.ItemID,
			AccessToken: res.AccessToken,
		}

		return pair, nil
	}
}

func (l *Linker) exchange(publicToken string) (plaid.ExchangePublicTokenResponse, error) {
	return l.Client.ExchangePublicToken(publicToken)
}

func NewLinker(client *plaid.Client) *Linker {
	return &Linker{
		Results: make(chan string),
		Errors:  make(chan error),
		Client:  client,
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
