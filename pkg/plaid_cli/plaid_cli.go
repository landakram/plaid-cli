package plaid_cli

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
)

type Data struct {
	DataDir     string
	Tokens      map[string]string
	Aliases     map[string]string
	BackAliases map[string]string
}

func LoadData(dataDir string) (*Data, error) {
	os.MkdirAll(filepath.Join(dataDir, "data"), os.ModePerm)

	data := &Data{
		DataDir:     dataDir,
		BackAliases: make(map[string]string),
	}

	data.loadTokens()
	data.loadAliases()

	return data, nil
}

func (d *Data) loadAliases() {
	var aliases map[string]string = make(map[string]string)
	filePath := d.aliasesPath()
	err := load(filePath, &aliases)
	if err != nil {
		log.Printf("Error loading aliases from %s. Assuming empty tokens. Error: %s", d.aliasesPath(), err)
	}

	d.Aliases = aliases

	for alias, itemID := range aliases {
		d.BackAliases[itemID] = alias
	}
}

func (d *Data) tokensPath() string {
	return filepath.Join(d.DataDir, "data", "tokens.json")
}

func (d *Data) aliasesPath() string {
	return filepath.Join(d.DataDir, "data", "aliases.json")
}

func (d *Data) loadTokens() {
	var tokens map[string]string = make(map[string]string)
	filePath := d.tokensPath()
	err := load(filePath, &tokens)
	if err != nil {
		log.Printf("Error loading tokens from %s. Assuming empty tokens. Error: %s", d.tokensPath(), err)
	}

	d.Tokens = tokens
}

func load(filePath string, v interface{}) error {
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0755)
	defer f.Close()

	if err != nil {
		return err
	} else {
		b, err := ioutil.ReadAll(f)
		if err != nil {
			return err
		}

		return json.Unmarshal(b, v)
	}
}

func (d *Data) Save() error {
	err := d.SaveTokens()
	if err != nil {
		return err
	}

	err = d.SaveAliases()
	if err != nil {
		return err
	}

	return nil
}

func (d *Data) SaveTokens() error {
	return save(d.Tokens, d.tokensPath())
}

func (d *Data) SaveAliases() error {
	return save(d.Aliases, d.aliasesPath())
}

func save(v interface{}, filePath string) error {
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	_, err = f.Write(b)
	return err
}
