package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/user"
	"path/filepath"

	"github.com/landakram/plaid-cli/pkg/plaid_cli"
	"github.com/plaid/plaid-go/plaid"
	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"

	"github.com/spf13/viper"
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
	usr, _ := user.Current()
	dir := usr.HomeDir
	viper.SetDefault("cli.data_dir", filepath.Join(dir, ".plaid-cli"))

	dataDir := viper.GetString("cli.data_dir")
	data := plaid_cli.LoadData(dataDir)

	viper.SetConfigName("config")
	viper.SetConfigType("toml")
	viper.AddConfigPath(dataDir)
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error if desired
		} else {
			log.Fatal(err)
		}
	}

	viper.AutomaticEnv()

	opts := plaid.ClientOptions{
		viper.GetString("plaid.client_id"),
		viper.GetString("plaid.secret"),
		viper.GetString("plaid.public_key"),
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

	linkCommand := &cobra.Command{
		Use:   "link",
		Short: "Link a bank account so plaid-cli can pull transactions.",
		Run: func(cmd *cobra.Command, args []string) {
			port := viper.Get("link.port")

			fmt.Println(fmt.Sprintf("Now listening on port %s", port))
			go func() {
				http.HandleFunc("/link", handleLink(linker))
				log.Fatal(http.ListenAndServe(fmt.Sprintf(":%s", port), nil))
			}()

			open.Run(fmt.Sprintf("http://localhost:%s/link", port))

			select {
			case err := <-linker.Errors:
				log.Fatal(err)
			case publicToken := <-linker.Results:
				res, err := linker.Link(publicToken)
				if err != nil {
					log.Fatal(err)
				}

				pair := plaid_cli.TokenPair{
					ItemID:      res.ItemID,
					AccessToken: res.AccessToken,
				}

				data.Tokens = append(data.Tokens, pair)
				err = data.Save()
				if err != nil {
					log.Fatalln(err)
				}
			}
		},
	}

	linkCommand.Flags().StringP("port", "p", "8080", "Port on which to serve Plaid Link")
	viper.BindPFlag("link.port", linkCommand.Flags().Lookup("port"))

	aliasCommand := &cobra.Command{
		Use:   "alias [ITEM-ID] [NAME]",
		Short: "Give a linked bank account a name.",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			itemID := args[0]
			name := args[1]

			data.Aliases[name] = itemID
			err = data.Save()
			if err != nil {
				log.Fatalln(err)
			}
		},
	}

	aliasesCommand := &cobra.Command{
		Use:   "aliases",
		Short: "List aliases",
		Run: func(cmd *cobra.Command, args []string) {
			printJSON, err := json.MarshalIndent(data.Aliases, "", "  ")
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(string(printJSON))
		},
	}

	rootCommand := &cobra.Command{Use: "plaid-cli"}
	rootCommand.AddCommand(linkCommand)
	rootCommand.AddCommand(aliasCommand)
	rootCommand.AddCommand(aliasesCommand)
	rootCommand.Execute()
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
