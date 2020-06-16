package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/landakram/plaid-cli/pkg/plaid_cli"
	"github.com/manifoldco/promptui"
	"github.com/plaid/plaid-go/plaid"
	"github.com/spf13/cobra"

	"github.com/spf13/viper"
)

func main() {
	log.SetFlags(0)

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

	viper.SetEnvPrefix("")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("plaid.environment", "development")
	plaidEnvStr := strings.ToLower(viper.GetString("plaid.environment"))

	var plaidEnv plaid.Environment
	switch plaidEnvStr {
	case "development":
		plaidEnv = plaid.Development
	case "production":
		plaidEnv = plaid.Production
	default:
		log.Fatalln("Invalid plaid environment. Valid plaid environments are 'development' or 'production'.")
	}

	opts := plaid.ClientOptions{
		viper.GetString("plaid.client_id"),
		viper.GetString("plaid.secret"),
		viper.GetString("plaid.public_key"),
		plaidEnv,
		&http.Client{},
	}

	client, err := plaid.NewClient(opts)

	if err != nil {
		log.Fatal(err)
	}

	linker := plaid_cli.NewLinker(data, client, opts)

	linkCommand := &cobra.Command{
		Use:   "link [ITEM-ID-OR-ALIAS]",
		Short: "Link an institution so plaid-cli can pull transactions",
		Long:  "Link an institution so plaid-cli can pull transactions. An item ID or alias can be passed to initiate a relink.",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			port := viper.GetString("link.port")

			var tokenPair *plaid_cli.TokenPair

			var err error

			if len(args) > 0 && len(args[0]) > 0 {
				itemOrAlias := args[0]

				itemID, ok := data.Aliases[itemOrAlias]
				if ok {
					itemOrAlias = itemID
				}

				tokenPair, err = linker.Relink(itemOrAlias, port)
			} else {
				tokenPair, err = linker.Link(port)
			}

			data.Tokens[tokenPair.ItemID] = tokenPair.AccessToken
			err = data.Save()
			if err != nil {
				log.Fatalln(err)
			}

			log.Println("Institution linked!")
			log.Println(fmt.Sprintf("Item ID: %s", tokenPair.ItemID))

			if alias, ok := data.BackAliases[tokenPair.ItemID]; ok {
				log.Println(fmt.Sprintf("Alias: %s", alias))
				return
			}

			validate := func(input string) error {
				matched, err := regexp.Match(`^\w+$`, []byte(input))
				if err != nil {
					return err
				}

				if !matched && input != "" {
					return errors.New("Valid characters: [0-9A-Za-z_]")
				}

				return nil
			}

			log.Println("You can give the institution a friendly alias and use that instead of the item ID in most commands.")
			prompt := promptui.Prompt{
				Label:    "Alias (default: none)",
				Validate: validate,
			}

			input, err := prompt.Run()
			if err != nil {
				log.Fatalln(err)
			}

			if input != "" {
				err = SetAlias(data, tokenPair.ItemID, input)
				if err != nil {
					log.Fatalln(err)
				}
			}
		},
	}

	linkCommand.Flags().StringP("port", "p", "8080", "Port on which to serve Plaid Link")
	viper.BindPFlag("link.port", linkCommand.Flags().Lookup("port"))

	tokensCommand := &cobra.Command{
		Use:   "tokens",
		Short: "List access tokens",
		Run: func(cmd *cobra.Command, args []string) {
			resolved := make(map[string]string)
			for itemID, token := range data.Tokens {
				if alias, ok := data.BackAliases[itemID]; ok {
					resolved[alias] = token
				} else {
					resolved[itemID] = token
				}
			}

			printJSON, err := json.MarshalIndent(resolved, "", "  ")
			if err != nil {
				log.Fatalln(err)
			}
			fmt.Println(string(printJSON))
		},
	}

	aliasCommand := &cobra.Command{
		Use:   "alias [ITEM-ID] [NAME]",
		Short: "Give a linked institution a friendly name",
		Long:  "Give a linked institution a friendly name. You can use this name instead of the idem ID in most commands.",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			itemID := args[0]
			alias := args[1]

			err := SetAlias(data, itemID, alias)
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
				log.Fatalln(err)
			}
			fmt.Println(string(printJSON))
		},
	}

	accountsCommand := &cobra.Command{
		Use:   "accounts [ITEM-ID-OR-ALIAS]",
		Short: "List accounts for a given institution",
		Long:  "List accounts for a given institution. An account ID returned from this command can be used as a filter when listing transactions.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			itemOrAlias := args[0]
			itemID, ok := data.Aliases[itemOrAlias]
			if ok {
				itemOrAlias = itemID
			}

			err := WithRelinkOnAuthError(itemOrAlias, data, linker, func() error {
				token := data.Tokens[itemOrAlias]
				res, err := client.GetAccounts(token)
				if err != nil {
					return err
				}

				b, err := json.MarshalIndent(res.Accounts, "", "  ")
				if err != nil {
					return err
				}

				fmt.Println(string(b))

				return nil
			})

			if err != nil {
				log.Fatalln(err)
			}
		},
	}

	var fromFlag string
	var toFlag string
	var accountID string
	var outputFormat string
	transactionsCommand := &cobra.Command{
		Use:   "transactions [ITEM-ID-OR-ALIAS]",
		Short: "List transactions for a given institution",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			itemOrAlias := args[0]
			itemID, ok := data.Aliases[itemOrAlias]
			if ok {
				itemOrAlias = itemID
			}

			err := WithRelinkOnAuthError(itemOrAlias, data, linker, func() error {
				token := data.Tokens[itemOrAlias]

				var accountIDs []string
				if len(accountID) > 0 {
					accountIDs = append(accountIDs, accountID)
				}

				options := plaid.GetTransactionsOptions{
					StartDate:  fromFlag,
					EndDate:    toFlag,
					AccountIDs: accountIDs,
					Count:      100,
					Offset:     0,
				}

				transactions, err := AllTransactions(options, client, token)
				if err != nil {
					return err
				}

				serializer, err := NewTransactionSerializer(outputFormat)
				if err != nil {
					return err
				}

				b, err := serializer.serialize(transactions)
				if err != nil {
					return err
				}

				fmt.Println(string(b))

				return nil
			})

			if err != nil {
				log.Fatalln(err)
			}
		},
	}
	transactionsCommand.Flags().StringVarP(&fromFlag, "from", "f", "", "Date of first transaction (required)")
	transactionsCommand.MarkFlagRequired("from")

	transactionsCommand.Flags().StringVarP(&toFlag, "to", "t", "", "Date of last transaction (required)")
	transactionsCommand.MarkFlagRequired("to")

	transactionsCommand.Flags().StringVarP(&outputFormat, "output-format", "o", "json", "Output format")
	transactionsCommand.Flags().StringVarP(&accountID, "account-id", "a", "", "Fetch transactions for this account ID only.")

	rootCommand := &cobra.Command{
		Use:   "plaid-cli",
		Short: "Link bank accounts and get transactions from the command line.",
		Long: `plaid-cli 🤑

plaid-cli is a CLI tool for working with the Plaid API.

You can use plaid-cli to link bank accounts and pull transactions in multiple 
output formats from the comfort of the command line.

Configuration:
  To get started, you'll need Plaid API credentials, which you can get by visiting
  https://dashboard.plaid.com/team/keys after signing up for free.
  
  plaid-cli will look at the following environment variables for API credentials:
  
    PLAID_CLIENT_ID=<client id>
    PLAID_PUBLIC_KEY=<public key>
    PLAID_SECRET=<devlopment secret>
    PLAID_ENVIRONMENT=development
  
  I recommend setting and exporting these on shell startup.
  
  API credentials can also be specified using a config file located at 
  ~/.plaid-cli/config.toml:
  
    [plaid]
    client_id = "<client id>"
    public_key = "<public key>"
    secret = "<development secret>"
    environment = "development"
  
  After setting those API credentials, plaid-cli is ready to use! 
  You'll probably want to run 'plaid-cli link' next.
  
  Please see the README (https://github.com/landakram/plaid-cli/blob/master/README.md) 
  for more detailed usage instructions.

  Made by @landakram.
`,
	}
	rootCommand.AddCommand(linkCommand)
	rootCommand.AddCommand(tokensCommand)
	rootCommand.AddCommand(aliasCommand)
	rootCommand.AddCommand(aliasesCommand)
	rootCommand.AddCommand(accountsCommand)
	rootCommand.AddCommand(transactionsCommand)

	if !viper.IsSet("plaid.client_id") {
		log.Println("⚠️  PLAID_CLIENT_ID not set. Please see the configuration instructions below.")
		rootCommand.Help()
		os.Exit(1)
	}
	if !viper.IsSet("plaid.secret") {
		log.Println("⚠️ PLAID_SECRET not set. Please see the configuration instructions below.")
		rootCommand.Help()
		os.Exit(1)
	}
	if !viper.IsSet("plaid.public_key") {
		log.Println("⚠️ PLAID_PUBLIC_KEY not set. Please see the configuration instructions below.")
		rootCommand.Help()
		os.Exit(1)
	}

	rootCommand.Execute()
}

func AllTransactions(opts plaid.GetTransactionsOptions, client *plaid.Client, token string) ([]plaid.Transaction, error) {
	var transactions []plaid.Transaction

	res, err := client.GetTransactionsWithOptions(token, opts)
	if err != nil {
		return transactions, err
	}

	transactions = append(transactions, res.Transactions...)

	for len(transactions) < res.TotalTransactions {
		opts.Offset += opts.Count
		res, err := client.GetTransactionsWithOptions(token, opts)
		if err != nil {
			return transactions, err
		}

		transactions = append(transactions, res.Transactions...)

	}

	return transactions, nil
}

func WithRelinkOnAuthError(itemID string, data *plaid_cli.Data, linker *plaid_cli.Linker, action func() error) error {
	err := action()
	if e, ok := err.(plaid.Error); ok {
		if e.ErrorCode == "ITEM_LOGIN_REQUIRED" {
			log.Println("Login expired. Relinking...")

			// TODO: this relink logic duplicated from the link command above

			port := viper.GetString("link.port")

			var tokenPair *plaid_cli.TokenPair

			tokenPair, err = linker.Relink(itemID, port)

			data.Tokens[tokenPair.ItemID] = tokenPair.AccessToken
			err = data.Save()
			if err != nil {
				return err
			}

			log.Println("Re-running action...")

			err = action()
		}
	}

	return err
}

type TransactionSerializer interface {
	serialize(txs []plaid.Transaction) ([]byte, error)
}

func NewTransactionSerializer(t string) (TransactionSerializer, error) {
	switch t {
	case "csv":
		return &CSVSerializer{}, nil
	case "json":
		return &JSONSerializer{}, nil
	default:
		return nil, errors.New(fmt.Sprintf("Invalid output format: %s", t))
	}
}

type CSVSerializer struct{}

func (w *CSVSerializer) serialize(txs []plaid.Transaction) ([]byte, error) {
	var records [][]string
	for _, tx := range txs {
		sanitizedName := strings.ReplaceAll(tx.Name, ",", "")
		records = append(records, []string{tx.Date, fmt.Sprintf("%f", tx.Amount), sanitizedName})
	}

	b := bytes.NewBufferString("")
	writer := csv.NewWriter(b)
	err := writer.Write([]string{"Date", "Amount", "Description"})
	if err != nil {
		return nil, err
	}
	err = writer.WriteAll(records)
	if err != nil {
		return nil, err
	}

	return b.Bytes(), err
}

func SetAlias(data *plaid_cli.Data, itemID string, alias string) error {
	if _, ok := data.Tokens[itemID]; !ok {
		return errors.New(fmt.Sprintf("No access token found for item ID `%s`. Try re-linking your account with `plaid-cli link`.", itemID))
	}

	data.Aliases[alias] = itemID
	data.BackAliases[itemID] = alias
	err := data.Save()
	if err != nil {
		return err
	}

	log.Println(fmt.Sprintf("Aliased %s to %s.", itemID, alias))

	return nil
}

type JSONSerializer struct{}

func (w *JSONSerializer) serialize(txs []plaid.Transaction) ([]byte, error) {
	return json.MarshalIndent(txs, "", "  ")
}
