package main

import (
	"fmt"
	"io"
	"log"
	"os"
)

func fatal(err error) {
	if err != nil {
		log.Fatal(fmt.Errorf("\u001b[31;1mFatal %v: %w\u001b[0m", err, err))
	}
}

func warn(message string) {
	l := log.New(os.Stderr, "", 0)
	l.Println("\u001b[33;1m" + message + "\u001b[0m")
}

func closer(closer io.Closer) {
	err := closer.Close()
	fatal(err)
}
