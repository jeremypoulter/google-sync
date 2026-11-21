// Command ldflags prints the -ldflags value that embeds an OAuth client into
// gmigrate, read from the client JSON downloaded from Google Cloud.
//
//	go run ./scripts/ldflags client_secret.json [version]
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const pkg = "github.com/jeremypoulter/google-sync/internal/config"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/ldflags CLIENT_JSON [VERSION]")
		os.Exit(2)
	}
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var f struct {
		Installed struct {
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
		} `json:"installed"`
	}
	if err := json.Unmarshal(b, &f); err != nil || f.Installed.ClientID == "" {
		fmt.Fprintln(os.Stderr, "not a Desktop app client JSON (expected an \"installed\" section)")
		os.Exit(1)
	}
	version := "dev"
	if len(os.Args) > 2 {
		version = os.Args[2]
	}
	fmt.Printf("-s -w -X %s.ClientID=%s -X %s.ClientSecret=%s -X %s.Version=%s",
		pkg, f.Installed.ClientID, pkg, f.Installed.ClientSecret, pkg, version)
}
