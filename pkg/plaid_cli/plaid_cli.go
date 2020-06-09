package plaid_cli

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
)

type Data struct {
	DataDir string
	Tokens  map[string]string
	Aliases map[string]string
}

func LoadData(dataDir string) *Data {
	os.MkdirAll(filepath.Join(dataDir, "data"), os.ModePerm)

	data := &Data{
		DataDir: dataDir,
	}
	data.loadTokens()
	data.loadAliases()
	return data
}

func (d *Data) loadAliases() error {
	var aliases map[string]string = make(map[string]string)
	filePath := d.aliasesPath()
	err := load(filePath, &aliases)
	if err != nil {
		return err
	}

	d.Aliases = aliases

	return nil
}

func (d *Data) tokensPath() string {
	return filepath.Join(d.DataDir, "data", "tokens.json")
}

func (d *Data) aliasesPath() string {
	return filepath.Join(d.DataDir, "data", "aliases.json")
}

func (d *Data) loadTokens() error {
	var tokens map[string]string
	filePath := d.tokensPath()
	err := load(filePath, &tokens)
	if err != nil {
		return err
	}

	d.Tokens = tokens

	return nil
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
