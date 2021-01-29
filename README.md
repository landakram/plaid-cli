# plaid-cli 🤑

> Link accounts and get transactions from the command line.

plaid-cli is a CLI tool for working with the Plaid API.

You can use plaid-cli to link bank accounts and pull transactions in multiple 
output formats from the comfort of the command line.

## Installation

Install with `go get`:

```sh
go get github.com/landakram/plaid-cli
```

Or grab a binary for your platform from the [Releases](https://github.com/landakram/plaid-cli/releases) page.

## Configuration

To get started, you'll need Plaid API credentials, which you can get by visiting
https://dashboard.plaid.com/team/keys after signing up for free.

plaid-cli will look at the following environment variables for API credentials:

```sh
PLAID_CLIENT_ID=<client id>
PLAID_SECRET=<devlopment secret>
PLAID_ENVIRONMENT=development
PLAID_LANGUAGE=en  # optional, detected using system's locale
PLAID_COUNTRIES=US # optional, detected using system's locale
```

I recommend setting and exporting these on shell startup.

API credentials can also be specified using a config file located at
~/.plaid-cli/config.toml:

```toml
[plaid]
client_id = "<client id>"
secret = "<development secret>"
environment = "development"
```

After setting those API credentials, plaid-cli is ready to use!
You'll probably want to run 'plaid-cli link' next.

## Usage 

<pre>
Usage:
  plaid-cli [command]

Available Commands:
  accounts     List accounts for a given institution
  alias        Give a linked bank account a name.
  aliases      List aliases
  help         Help about any command
  link         Link a bank account so plaid-cli can pull transactions.
  tokens       List tokens
  transactions List transactions for a given account

Flags:
  -h, --help   help for plaid-cli

Use "plaid-cli [command] --help" for more information about a command.
</pre>

### Link an account

Run:

```
plaid-cli link
```

plaid-cli will start a webserver and open your browser so you can link your bank account 
with [Plaid Link](https://blog.plaid.com/plaid-link/). 

To see the access token you just created and the "Plaid Item ID" it's associated with,
you can run:

```
plaid-cli tokens
```

### Alias a link

You can make human-readable names for a linked instituion by running:

```
plaid-cli alias <long-alphanumeric-item-id> nice-name
```

You can now refer to the linked instituion by `nice-name` in most commands.

### Pulling transactions

You can pull transaction history for an institution by running:

```
plaid-cli transactions <item-id-or-alias> --from 2020-06-01 --to 2020-06-10 --output-format csv > out.csv
```

The output is suitable for manual import in budgeting tools such as YNAB.

### Relinking

Most commands will prompt you to relink automatically if your bank login has expired (due to 2FA, for example). 

To manually relink, you can run the link command with an item ID or alias:

```
plaid-cli link nice-name
```

## Why

I wanted to work around YNAB's flaky direct import feature. For some reason, it's not able
to sync transactions with SoFi and SoFi only provides a PDF statement history unsuitable for
manual import.

Similar projects:

* [plaid2qif](https://github.com/ebridges/plaid2qif). A very similar Python-based cli tool. The major difference is that plaid-cli handles linking to account automatically and will prompt for relinks.
