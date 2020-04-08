package main

import (
	"fmt"
	"os/exec"
)

func shell(command string) {
	cmd := exec.Command("sh", "-c", command)
	stdoutStderr, err := cmd.CombinedOutput()
	fatal(err)
	fmt.Printf("%s\n", stdoutStderr)
}
