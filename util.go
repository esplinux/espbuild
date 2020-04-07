package main

import (
	"io"
	"log"
)

func fatal(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func closer(closer io.Closer) {
	err := closer.Close()
	fatal(err)
}
