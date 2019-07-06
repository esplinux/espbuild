package main

import (
  "gopkg.in/src-d/go-git.v4"
  "gopkg.in/src-d/go-git.v4/plumbing"
)

import "log"
import "os"

func clone(destination string, url string) error {
	log.Print("Internal clone: " + url)
	_, err := git.PlainClone(destination, false, &git.CloneOptions{
		URL:      url,
		Progress: os.Stdout,
	})

	return err
}

func cloneBranch(destination string, url string, branch string) error {
	log.Print("Internal clone: " + url + " branch: " + branch)

	_, err := git.PlainClone(destination, false, &git.CloneOptions{
		URL:      url,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch: true,
		Progress: os.Stdout,
	})

	return err
}

func cloneTag(destination string, url string, tag string) error {
	log.Print("Internal clone: " + url + " tag: " + tag)

	_, err := git.PlainClone(destination, false, &git.CloneOptions{
		URL:      url,
		ReferenceName: plumbing.NewTagReferenceName(tag),
		SingleBranch: true,
		Progress: os.Stdout,
	})

	return err
}

func cloneCheckout(destination string, url string, commit string) error {
	log.Print("Internal clone: " + url + " commit: " + commit)
	ref, err := git.PlainClone(destination, false, &git.CloneOptions{
		URL:      url,
		Progress: os.Stdout,
	})
	if err != nil {
		return err
	}

	wt, err := ref.Worktree()
	if err != nil {
		return err
	}

	// ... checking out to commit
	err = wt.Checkout(&git.CheckoutOptions{
		Hash: plumbing.NewHash(commit),
	})

	return err
}
