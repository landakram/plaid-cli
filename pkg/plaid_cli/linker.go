package plaid_cli

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"text/template"

	"github.com/plaid/plaid-go/plaid"
	"github.com/skratchdot/open-golang/open"
)

type Linker struct {
	Results chan string
	Errors  chan error
	Client  *plaid.Client
	Data    *Data
}

type TokenPair struct {
	ItemID      string
	AccessToken string
}

func (l *Linker) Relink(itemID string, port string) (*TokenPair, error) {
	token := l.Data.Tokens[itemID]
	res, err := l.Client.CreatePublicToken(token)
	if err != nil {
		return nil, err
	}

	return l.link(port, handleRelink(l, res.PublicToken))
}

func (l *Linker) Link(port string) (*TokenPair, error) {
	return l.link(port, handleLink(l))
}

func (l *Linker) link(port string, serveLink func(w http.ResponseWriter, r *http.Request)) (*TokenPair, error) {
	log.Println(fmt.Sprintf("Starting Plaid Link on port %s", port))
	go func() {
		http.HandleFunc("/link", serveLink)
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

func NewLinker(data *Data, client *plaid.Client) *Linker {
	return &Linker{
		Results: make(chan string),
		Errors:  make(chan error),
		Client:  client,
		Data:    data,
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

			fmt.Fprintf(w, "ok")
		default:
			linker.Errors <- errors.New("Invalid HTTP method")
		}
	}
}

type RelinkTemplData struct {
	PublicToken string
}

func handleRelink(linker *Linker, publicToken string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			t := template.New("relink")
			t, _ = t.Parse(relinkTemplate)
			d := RelinkTemplData{
				PublicToken: publicToken,
			}
			t.Execute(w, d)
		case http.MethodPost:
			r.ParseForm()
			token := r.Form.Get("public_token")
			if token != "" {
				linker.Results <- token
			} else {
				linker.Errors <- errors.New("Empty public_token")
			}

			fmt.Fprintf(w, "ok")
		default:
			linker.Errors <- errors.New("Invalid HTTP method")
		}
	}
}

var relinkTemplate string = `<html>
  <body>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/2.2.3/jquery.min.js"></script>
    <script src="https://cdn.plaid.com/link/v2/stable/link-initialize.js"></script>
    <script type="text/javascript">
     (function($) {
       var handler = Plaid.create({
         clientName: 'plaid-cli',
         // Optional, specify an array of ISO-3166-1 alpha-2 country
         // codes to initialize Link; European countries will have GDPR
         // consent panel
         countryCodes: ['US'],
         env: 'development',
         // Replace with your public_key from the Dashboard
         key: '880bb11f8bc9f3c1d8feb4a348f371',
         product: ['transactions'],
         token: "{{ .PublicToken }}",
         language: 'en',
         onLoad: function() {
           // Optional, called when Link loads
         },
         onSuccess: function(public_token, metadata) {
           // Send the public_token to your app server.
           // The metadata object contains info about the institution the
           // user selected and the account ID or IDs, if the
           // Select Account view is enabled.
           $.post('/link', {
             public_token: public_token,
           });
         },
         onExit: function(err, metadata) {
           // The user exited the Link flow.
           if (err != null) {
             // The user encountered a Plaid API error prior to exiting.
           }
           // metadata contains information about the institution
           // that the user selected and the most recent API request IDs.
           // Storing this information can be helpful for support.
         },
         onEvent: function(eventName, metadata) {
           // Optionally capture Link flow events, streamed through
           // this callback as your users connect an Item to Plaid.
           // For example:
           // eventName = "TRANSITION_VIEW"
           // metadata  = {
           //   link_session_id: "123-abc",
           //   mfa_type:        "questions",
           //   timestamp:       "2017-09-14T14:42:19.350Z",
           //   view_name:       "MFA",
           // }
         }
       });

       handler.open();

     })(jQuery);
    </script>
  </body>
</html>`
