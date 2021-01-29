package plaid_cli

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"text/template"

	"github.com/plaid/plaid-go/plaid"
	"github.com/skratchdot/open-golang/open"
)

type Linker struct {
	Results       chan string
	RelinkResults chan bool
	Errors        chan error
	Client        *plaid.Client
	Data          *Data
	countries     []string
	lang          string
}

type TokenPair struct {
	ItemID      string
	AccessToken string
}

func (l *Linker) Relink(itemID string, port string) error {
	token := l.Data.Tokens[itemID]
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatal(err)
	}
	resp, err := l.Client.CreateLinkToken(plaid.LinkTokenConfigs{
		User: &plaid.LinkTokenUser{
			ClientUserID: hostname,
		},
		ClientName:   "plaid-cli",
		CountryCodes: l.countries,
		Language:     l.lang,
		AccessToken:  token,
	})
	if err != nil {
		log.Fatal(err)
	}
	return l.relink(port, resp.LinkToken)
}

func (l *Linker) Link(port string) (*TokenPair, error) {
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatal(err)
	}
	resp, err := l.Client.CreateLinkToken(plaid.LinkTokenConfigs{
		User: &plaid.LinkTokenUser{
			ClientUserID: hostname,
		},
		ClientName:   "plaid-cli",
		Products:     []string{"transactions"},
		CountryCodes: l.countries,
		Language:     l.lang,
	})
	if err != nil {
		log.Fatal(err)
	}
	return l.link(port, resp.LinkToken)
}

func (l *Linker) link(port string, linkToken string) (*TokenPair, error) {
	log.Println(fmt.Sprintf("Starting Plaid Link on port %s...", port))

	go func() {
		http.HandleFunc("/link", handleLink(l, linkToken))
		err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
		if err != nil {
			l.Errors <- err
		}
	}()

	url := fmt.Sprintf("http://localhost:%s/link", port)
	log.Println(fmt.Sprintf("Your browser should open automatically. If it doesn't, please visit %s to continue linking!", url))
	open.Run(url)

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

func (l *Linker) relink(port string, linkToken string) error {
	log.Println(fmt.Sprintf("Starting Plaid Link on port %s...", port))

	go func() {
		http.HandleFunc("/relink", handleRelink(l, linkToken))
		err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
		if err != nil {
			l.Errors <- err
		}
	}()

	url := fmt.Sprintf("http://localhost:%s/relink", port)
	log.Println(fmt.Sprintf("Your browser should open automatically. If it doesn't, please visit %s to continue linking!", url))
	open.Run(url)

	select {
	case err := <-l.Errors:
		return err
	case <-l.RelinkResults:
		return nil
	}
}

func (l *Linker) exchange(publicToken string) (plaid.ExchangePublicTokenResponse, error) {
	return l.Client.ExchangePublicToken(publicToken)
}

func NewLinker(data *Data, client *plaid.Client, countries []string, lang string) *Linker {
	return &Linker{
		Results:       make(chan string),
		RelinkResults: make(chan bool),
		Errors:        make(chan error),
		Client:        client,
		Data:          data,
		countries:     countries,
		lang:          lang,
	}
}

func handleLink(linker *Linker, linkToken string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			t := template.New("link")
			t, _ = t.Parse(linkTemplate)

			d := LinkTmplData{
				LinkToken: linkToken,
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

type LinkTmplData struct {
	LinkToken string
}

type RelinkTmplData struct {
	LinkToken string
}

func handleRelink(linker *Linker, linkToken string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			t := template.New("relink")
			t, _ = t.Parse(relinkTemplate)

			d := RelinkTmplData{
				LinkToken: linkToken,
			}
			t.Execute(w, d)
		case http.MethodPost:
			r.ParseForm()
			err := r.Form.Get("error")
			if err != "" {
				linker.Errors <- errors.New(err)
			} else {
				linker.RelinkResults <- true
			}

			fmt.Fprintf(w, "ok")
		default:
			linker.Errors <- errors.New("Invalid HTTP method")
		}
	}
}

var linkTemplate string = `<html>
  <head>
    <style>
    .alert-success {
	font-size: 1.2em;
	font-family: Arial, Helvetica, sans-serif;
	background-color: #008000;
	color: #fff;
	display: flex;
	justify-content: center;
	align-items: center;
	border-radius: 15px;
	width: 100%;
	height: 100%;
    }
    .hidden {
	visibility: hidden;
    }
    </style>
  </head>
  <body>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/2.2.3/jquery.min.js"></script>
    <script src="https://cdn.plaid.com/link/v2/stable/link-initialize.js"></script>
    <script type="text/javascript">
     (function($) {
       var handler = Plaid.create({
	 token: '{{ .LinkToken }}',
	 onSuccess: function(public_token, metadata) {
	   // Send the public_token to your app server.
	   // The metadata object contains info about the institution the
	   // user selected and the account ID or IDs, if the
	   // Select Account view is enabled.
	   $.post('/link', {
	     public_token: public_token,
	   });
	   document.getElementById("alert").classList.remove("hidden");
	 },
	 onExit: function(err, metadata) {
	   // The user exited the Link flow.
	   if (err != null) {
	     // The user encountered a Plaid API error prior to exiting.
	   }
	   // metadata contains information about the institution
	   // that the user selected and the most recent API request IDs.
	   // Storing this information can be helpful for support.

	   document.getElementById("alert").classList.remove("hidden");
	 }
       });

       handler.open();

     })(jQuery);
    </script>

    <div id="alert" class="alert-success hidden">
      <div>
	<h2>All done here!</h2>
	<p>You can close this window and go back to plaid-cli.</p>
      </div>
    </div>
  </body>
</html> `

var relinkTemplate string = `<html>
  <head>
    <style>
    .alert-success {
	font-size: 1.2em;
	font-family: Arial, Helvetica, sans-serif;
	background-color: #008000;
	color: #fff;
	display: flex;
	justify-content: center;
	align-items: center;
	border-radius: 15px;
	width: 100%;
	height: 100%;
    }
    .hidden {
	visibility: hidden;
    }
    </style>
  </head>
  <body>
    <script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/2.2.3/jquery.min.js"></script>
    <script src="https://cdn.plaid.com/link/v2/stable/link-initialize.js"></script>
    <script type="text/javascript">
     (function($) {
       var handler = Plaid.create({
	 token: '{{ .LinkToken }}',
	 onSuccess: (public_token, metadata) => {
	   // You do not need to repeat the /item/public_token/exchange
	   // process when a user uses Link in update mode.
	   // The Item's access_token has not changed.
	 },
	 onExit: function(err, metadata) {
	   if (err != null) {
	     $.post('/relink', {
	       error: err
	     });
	   } else {
	     $.post('/relink', {
	       error: null
	     });
	   }
	   // metadata contains information about the institution
	   // that the user selected and the most recent API request IDs.
	   // Storing this information can be helpful for support.

	   document.getElementById("alert").classList.remove("hidden");
	 }
       });

       handler.open();

     })(jQuery);
    </script>

    <div id="alert" class="alert-success hidden">
      <div>
	<h2>All done here!</h2>
	<p>You can close this window and go back to plaid-cli.</p>
      </div>
    </div>
  </body>
</html>`
