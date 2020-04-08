package main

import (
	"fmt"
	"log"
	"os"
)

const ESP_BUILD_VERSION = "0.0.1"

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("\tespbuild package.esp")
	} else {
		cache := &cache{
			cache: make(map[string]*entry),
		}

		ch := make(chan string)
		go func(buildfile string) {
			globals, err := cache.Load(buildfile)
			if err != nil {
				log.Fatal(err)
			}
			ch <- fmt.Sprintf("%s = %s", buildfile, globals)
		}(os.Args[1])
		println("Globals: " + <-ch)
	}
}
