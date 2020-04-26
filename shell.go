package main

import (
	"bytes"
	"fmt"
	"go.starlark.net/starlark"
	"io"
	"os"
	"os/exec"
)

func shell(command string, quiet bool, env *starlark.Dict) (starlark.Value, error) {
	cmd := exec.Command("sh", "-c", command)
	envList := os.Environ()

	iter := env.Iterate()
	defer iter.Done()
	var k starlark.Value
	for iter.Next(&k) {
		v, _, err := env.Get(k)
		if err != nil {
			return starlark.None, err
		}

		key, _ := starlark.AsString(k)
		value, _ := starlark.AsString(v)

		envList = append(envList, key+"="+value)
	}
	cmd.Env = envList

	var outBuf, errBuf bytes.Buffer
	if quiet {
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
	} else {
		cmd.Stdout = io.MultiWriter(os.Stdout, &outBuf)
		cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)
	}
	if err := cmd.Run(); err != nil {
		return starlark.String(outBuf.String()), fmt.Errorf("shell(%v): %s", err, errBuf.String())
	}
	return starlark.String(outBuf.String()), nil
}
