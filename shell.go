package main

import (
	"bytes"
	"fmt"
	"go.starlark.net/starlark"
	"os"
	"os/exec"
)

func shell(command string, env *starlark.Dict) error {
	cmd := exec.Command("sh", "-c", command)
	envList := os.Environ()

	iter := env.Iterate()
	defer iter.Done()
	var k starlark.Value
	for iter.Next(&k) {
		v, _, err := env.Get(k)
		if err != nil {
			return err
		}

		key, _ := starlark.AsString(k)
		value, _ := starlark.AsString(v)

		envList = append(envList, key+"="+value)
	}
	cmd.Env = envList

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	stdoutBuf := new(bytes.Buffer)
	if _, err :=  stdoutBuf.ReadFrom(stdout); err != nil {
		return err
	}

	stderrBuf := new(bytes.Buffer)
	if _, err = stderrBuf.ReadFrom(stderr); err != nil {
		return err
	}

	if err := stdout.Close(); err != nil {
		return err
	}

	if err := stderr.Close(); err != nil {
		return err
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("shell(%v): %s", err, stderrBuf.String())
	}
	
	print(stdoutBuf.String())
	return nil
}
