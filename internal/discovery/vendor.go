package discovery

//go:generate go run gen_oui.go oui.csv

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"strconv"
	"strings"
	"sync"
)

// oui.txt.gz maps the first half of a MAC address to the company that owns
// it, from the IEEE registry. gen_oui.go says how to refresh it.
//
//go:embed oui.txt.gz
var ouiGz []byte

var ouiTable = sync.OnceValue(func() map[string]string {
	m := map[string]string{}
	r, err := gzip.NewReader(bytes.NewReader(ouiGz))
	if err != nil {
		return m
	}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if prefix, org, ok := strings.Cut(sc.Text(), "\t"); ok {
			m[prefix] = org
		}
	}
	return m
})

// Vendor names who made a device from its MAC address ("aa:bb:cc:dd:ee:ff"),
// the way people say it: "Espressif", not "Espressif Inc.". It's empty for
// unknown and private addresses.
func Vendor(mac string) string {
	if len(mac) < 8 || Private(mac) {
		return ""
	}
	return maker(ouiTable()[strings.ToUpper(strings.ReplaceAll(mac[:8], ":", ""))])
}

// Private reports a random address that phones and computers pick for each
// network. It names no maker.
func Private(mac string) bool {
	if len(mac) < 2 {
		return false
	}
	b, err := strconv.ParseUint(mac[:2], 16, 8)
	return err == nil && b&2 != 0
}

// Words that end the brand part of a registered company name.
var corpWords = map[string]bool{
	"inc": true, "co": true, "ltd": true, "corp": true, "corporation": true, "corporate": true, "company": true,
	"limited": true, "llc": true, "gmbh": true, "bv": true, "sa": true, "sas": true, "ag": true, "plc": true,
	"technologies": true, "technology": true, "tech": true, "electronics": true, "electronic": true,
	"electro-mechanics": true, "communications": true, "communication": true, "international": true,
	"industrial": true, "industries": true, "ind": true, "precision": true, "manufacturing": true,
	"trading": true, "information": true, "intelligent": true, "smart": true, "digital": true,
	"systems": true, "devices": true, "device": true, "group": true, "holdings": true, "mobile": true,
	"mobility": true, "interactive": true, "computer": true, "network": true, "networks": true, "wireless": true,
	"broadband": true, "telecom": true, "telecommunication": true, "telecommunications": true, "usa": true,
	"ab": true, "oy": true,
}

// Where many makers are registered. "Shenzhen Bilian" is better as "Bilian".
var places = map[string]bool{
	"shenzhen": true, "guangdong": true, "guangzhou": true, "hangzhou": true, "beijing": true, "shanghai": true,
	"zhejiang": true, "dongguan": true, "xiamen": true, "suzhou": true, "fujian": true, "jiangsu": true,
	"qingdao": true, "zhuhai": true, "wuhan": true, "chengdu": true, "nanjing": true, "ningbo": true,
}

// Brands people know better than the registered company.
var brands = map[string]string{
	"TP-LINK": "TP-Link", "Hon Hai": "Foxconn", "Signify": "Philips Hue", "Allterco Robotics": "Shelly",
	"ASUSTek": "Asus", "zte": "ZTE", "China": "China Mobile",
	"Ieee Registration": "", "Private": "", // blocks resold in smaller parts, or hidden: no maker
}

// maker shortens a registered company name to its brand:
// "TP-LINK TECHNOLOGIES CO.,LTD." becomes "TP-Link".
func maker(org string) string {
	if i := strings.IndexAny(org, ",(（，"); i > 0 {
		org = org[:i]
	}
	words := strings.Fields(org)
	for len(words) > 1 && places[strings.ToLower(words[0])] {
		words = words[1:]
	}
	var keep []string
	for _, w := range words {
		if corpWords[strings.ToLower(strings.ReplaceAll(w, ".", ""))] {
			break
		}
		keep = append(keep, w)
	}
	if len(keep) == 0 && len(words) > 0 {
		keep = words[:1]
	}
	if len(keep) > 2 {
		keep = keep[:2]
	}
	for i, w := range keep {
		// "SAMSUNG" reads better as "Samsung". Short ones like "LG" stay.
		if len(w) > 3 && w == strings.ToUpper(w) && !strings.ContainsAny(w, "-&0123456789") {
			keep[i] = w[:1] + strings.ToLower(w[1:])
		}
	}
	name := strings.Join(keep, " ")
	if b, ok := brands[name]; ok {
		return b
	}
	return name
}
