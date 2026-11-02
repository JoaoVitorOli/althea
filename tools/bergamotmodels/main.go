// Command bergamotmodels pins the Translator's offline models: for every pair of languages, the
// address, size and sha256 of each file of its `tiny` model, from the records Mozilla publishes for
// Firefox's translator. The app only ever downloads what this file names, and checks it.
//
//	curl -o records.json https://firefox.settings.services.mozilla.com/v1/buckets/main/collections/translations-models/records
//	go -C tools run ./bergamotmodels -records records.json -out ../extensions/translator/models.json
//
// Version 1.0 of the records is the `tiny` architecture, 17 MB a model; later versions are larger.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
)

const (
	version     = "1.0"
	attachments = "https://firefox-settings-attachments.cdn.mozilla.net/"
)

type record struct {
	FromLang   string `json:"fromLang"`
	ToLang     string `json:"toLang"`
	Version    string `json:"version"`
	FileType   string `json:"fileType"`
	Attachment struct {
		Hash     string `json:"hash"`
		Size     int64  `json:"size"`
		Location string `json:"location"`
	} `json:"attachment"`
}

// File is a file of a model, as the extension reads it.
type File struct {
	Role   string `json:"role"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Pair is the model of one direction, as the extension reads it.
type Pair struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Files []File `json:"files"`
}

func main() {
	in := flag.String("records", "records.json", "the collection's records")
	out := flag.String("out", "models.json", "the file to write")
	flag.Parse()
	if err := run(*in, *out); err != nil {
		log.Fatal(err)
	}
}

func run(in, out string) error {
	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	var doc struct {
		Data []record `json:"data"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	byPair := map[string]*Pair{}
	for _, r := range doc.Data {
		if r.Version != version {
			continue
		}
		key := r.FromLang + "-" + r.ToLang
		p := byPair[key]
		if p == nil {
			p = &Pair{From: r.FromLang, To: r.ToLang}
			byPair[key] = p
		}
		p.Files = append(p.Files, File{r.FileType, attachments + r.Attachment.Location, r.Attachment.Size, r.Attachment.Hash})
	}
	pairs := make([]Pair, 0, len(byPair))
	for key, p := range byPair {
		slices.SortFunc(p.Files, func(a, b File) int {
			switch {
			case a.Role < b.Role:
				return -1
			case a.Role > b.Role:
				return 1
			}
			return 0
		})
		roles := make([]string, len(p.Files))
		for i, f := range p.Files {
			roles[i] = f.Role
		}
		if !slices.Equal(roles, []string{"lex", "model", "vocab"}) {
			return fmt.Errorf("%s has files %v, not one each of lex, model and vocab", key, roles)
		}
		pairs = append(pairs, *p)
	}
	slices.SortFunc(pairs, func(a, b Pair) int {
		switch x, y := a.From+"-"+a.To, b.From+"-"+b.To; {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	})
	encoded, err := json.MarshalIndent(pairs, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(out, append(encoded, '\n'), 0o600)
}
