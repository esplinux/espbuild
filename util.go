package main

import (
	"fmt"
	"log"
	"os"
)

func debug(message string) {
	if DEBUG {
		log.Printf("\u001b[31;1mDebug: %s\u001b[0m", message)
	}
}

func fatal(err error) {
	if err != nil {
		log.Fatal(fmt.Errorf("\u001b[31;1mFatal %v: %w\u001b[0m", err, err))
	}
}

func warn(message string) {
	l := log.New(os.Stderr, "", 0)
	l.Println("\u001b[33;1m" + message + "\u001b[0m")
}
