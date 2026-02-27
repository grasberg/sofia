package main

import (
	"fmt"
	"github.com/grasberg/sofia/pkg/config"
	"os"
)

func main() {
	home, _ := os.UserHomeDir()
	path := home + "/.sofia/config.json"
	cfg, err := config.LoadConfig(path)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Enabled: %v\n", cfg.WebUI.Enabled)
	fmt.Printf("Host: %s\n", cfg.WebUI.Host)
	fmt.Printf("Port: %d\n", cfg.WebUI.Port)
}
