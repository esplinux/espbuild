package main

import (
	"fmt"
	"go.starlark.net/starlark"
	"os"
	"os/exec"
)

func shell(command string, env *starlark.Dict) {
	cmd := exec.Command("sh", "-c", command)
	envList := os.Environ()

	iter := env.Iterate()
	defer iter.Done()
	var k starlark.Value
	for iter.Next(&k) {
		v, _, err := env.Get(k)
		fatal(err)

		key, _ := starlark.AsString(k)
		value, _ := starlark.AsString(v)

		envList = append(envList, key+"="+value)
	}
	cmd.Env = envList
	stdoutStderr, err := cmd.CombinedOutput()
	fatal(err)
	fmt.Printf("%s\n", stdoutStderr)
}
