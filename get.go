package main

import "fmt"
import "io"
import "log"
import "net"
import "net/http"
import "os"
import "strings"
import "time"

import "gopkg.in/src-d/go-git.v4"

func getHttp(url string) {
	urlSplit := strings.Split(url, "/")
	outputFile := urlSplit[len(urlSplit)-1]

	var netTransport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
	}

	var netClient = &http.Client{
		Timeout:   time.Second * 10,
		Transport: netTransport,
	}

	resp, err := netClient.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	out, err := os.Create(outputFile)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	io.Copy(out, resp.Body)
}

func getGit(url string, outputDir string) {
	_, err := git.PlainClone(outputDir, false, &git.CloneOptions{
		URL: url,
	})
	if err != nil {
		log.Fatal(err)
	}
}

func getGitInferred(url string) {
	urlSplit := strings.Split(url, "/")
	outputDir := urlSplit[len(urlSplit)-1]
	getGit(url, outputDir)
}

func main() {
	//getHttp(os.Args[1], os.Args[2])

	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("\tget https://example.com/filename.tar.gz")
		fmt.Println("\tget http https://example.com/filename.tar.gz")
		fmt.Println("\tget git https://exmample.com/git/url")
	} else {
		switch os.Args[1] {
		case "git":
			if len(os.Args) > 3 {
				getGit(os.Args[2], os.Args[3])
			} else {
				getGitInferred(os.Args[2])
			}
		default:
			if os.Args[1] == "http" {
				getHttp(os.Args[2])
			} else {
				getHttp(os.Args[1])
			}
		}
	}
}
