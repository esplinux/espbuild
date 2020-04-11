package main

import (
	"errors"
	"fmt"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
)

func getPredeclared() starlark.StringDict {
	pathBuiltIn := func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		buildfile, err := filepath.Abs(thread.Name)
		if err != nil {
			return starlark.None, err
		}
		curdir := filepath.Dir(buildfile)

		var path string
		if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
			return nil, err
		}

		return starlark.String(curdir + "/" + path), nil
	}

	shellBuiltIn := func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		env := starlark.NewDict(3)
		var command string
		if err := starlark.UnpackArgs(b.Name(), args, kwargs, "command", &command, "env?", &env); err != nil {
			return nil, err
		}
		println(thread.Name + ": " + command)
		return starlark.None, shell(command, env)
	}

	sourceBuiltIn := func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		buildfile, err := filepath.Abs(thread.Name)
		if err != nil {
			return starlark.None, err
		}
		curdir := filepath.Dir(buildfile)

		http := ""
		git := ""
		branch := ""
		if err := starlark.UnpackArgs(b.Name(), args, kwargs, "http?", &http, "git?", &git, "branch?", &branch); err != nil {
			return nil, err
		}

		if http != "" {
			return getHttpSource(http, curdir)
		} else if git != "" {
			return getGit(git, branch, curdir)
		} else {
			return starlark.None, errors.New("source only supports git and http")
		}
	}

	predeclared := starlark.StringDict{
		"NPROC":   starlark.String(strconv.Itoa(runtime.NumCPU())),
		"path":    starlark.NewBuiltin("path", pathBuiltIn),
		"shell":   starlark.NewBuiltin("shell", shellBuiltIn),
		"source":  starlark.NewBuiltin("source", sourceBuiltIn),
		"struct":  starlark.NewBuiltin("struct", starlarkstruct.Make),
		"package": starlark.NewBuiltin("package", starlarkstruct.Make),
	}

	return predeclared
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("\tespbuild package.esp")
	} else {
		predeclared := getPredeclared()

		globals, err := starlark.ExecFile(&starlark.Thread{Name: "common"}, "common.esp", nil, predeclared)
		fatal(err)

		for k, v := range globals {
			predeclared[k] = v
		}

		cache := &cache{
			cache:       make(map[string]*entry),
			predeclared: predeclared,
		}

		ch := make(chan string)
		for _, arg := range os.Args[1:] {
			go func(buildfile string) {
				globals, err := cache.Load(buildfile)
				if err != nil {
					log.Fatal(err)
				}
				ch <- fmt.Sprintf("%s = %s", buildfile, globals)
			}(arg)
		}

		for _, arg := range os.Args[1:] {
			println("Globals[" + arg + "]: " + <-ch)
		}
	}
}
