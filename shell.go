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

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		print(outBuf.String())
		return fmt.Errorf("shell(%v): %s", err, errBuf.String())
	}
	print(outBuf.String())

	return nil
}
