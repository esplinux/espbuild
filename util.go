package main

import (
	"io"
	"log"
	"os"
)

func fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func warn(message string) {
	l := log.New(os.Stderr, "", 0)
	l.Println(message)
}

func closer(closer io.Closer) {
	err := closer.Close()
	fatal(err)
}
