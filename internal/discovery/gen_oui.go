//go:build ignore

// gen_oui turns the IEEE registry of MAC prefixes into oui.txt.gz: one
// "AABBCC<tab>Company" line per prefix. To refresh it, download
// https://standards-oui.ieee.org/oui/oui.csv in a browser (the site turns
// scripts away), put it in this folder, then run: go generate ./internal/discovery
package main

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		log.Fatal(err)
	}
	var lines []string
	for _, r := range rows {
		// MA-L is the large blocks consumer makers use. Columns: registry, prefix, company, address.
		if len(r) >= 3 && r[0] == "MA-L" {
			lines = append(lines, r[1]+"\t"+strings.Join(strings.Fields(r[2]), " "))
		}
	}
	sort.Strings(lines)
	out, err := os.Create("oui.txt.gz")
	if err != nil {
		log.Fatal(err)
	}
	zw, _ := gzip.NewWriterLevel(out, gzip.BestCompression)
	fmt.Fprintln(zw, strings.Join(lines, "\n"))
	if err := zw.Close(); err != nil {
		log.Fatal(err)
	}
	out.Close()
	fmt.Printf("oui.txt.gz: %d prefixes\n", len(lines))
}
