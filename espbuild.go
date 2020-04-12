package main

import (
	"errors"
	"fmt"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go/build"
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

	fetchBuiltIn := func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		buildfile, err := filepath.Abs(thread.Name)
		if err != nil {
			return starlark.None, err
		}
		curdir := filepath.Dir(buildfile)

		http := ""
		git := ""
		branch := ""
		file := ""
		if err := starlark.UnpackArgs(b.Name(), args, kwargs, "http?", &http, "file?", &file, "git?", &git, "branch?", &branch); err != nil {
			return nil, err
		}

		if http != "" {
			if (file != "") {
				return getHttpFile(http, curdir, file)
			} else {
				return getHttpSource(http, curdir)
			}
		} else if git != "" {
			return getGit(git, branch, curdir)
		} else {
			return starlark.None, errors.New("source only supports git and http")
		}
	}

	predeclared := starlark.StringDict{
		"NPROC":  starlark.String(strconv.Itoa(runtime.NumCPU())),
		"path":   starlark.NewBuiltin("path", pathBuiltIn),
		"shell":  starlark.NewBuiltin("shell", shellBuiltIn),
		"fetch":  starlark.NewBuiltin("fetch", fetchBuiltIn),
		"struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
	}

	return predeclared
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func main() {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = build.Default.GOPATH
	}

	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("\tespbuild package.esp")
	} else {
		var builtins string
		if fileExists("builtins.esp") {
			builtins = "builtins.esp"
		} else if fileExists("/share/esp/builtins.esp") {
			builtins = "/share/esp/builtins.esp"
		} else if fileExists("/usr/share/esp/builtins.esp") {
			builtins = "/usr/share/esp/builtins.esp"
		} else if fileExists(gopath + "/src/github.com/esplinux/espbuild/builtins.esp") {
			builtins = gopath + "/src/github.com/esplinux/espbuild/builtins.esp"
		}

		if builtins == "" {
			log.Fatal("Could not find builtins.esp")
		}

		predeclared := getPredeclared()
		globals, err := starlark.ExecFile(&starlark.Thread{Name: builtins}, builtins, nil, predeclared)
		fatal(err)

		for k, v := range globals {
			predeclared[k] = v
		}

		args := os.Args[1:]
		if os.Args[1] == "-D" {
			if len(os.Args) < 3 {
				log.Fatal("You must specify a parameter with -D")
			} else {
				predeclared[os.Args[2]] = starlark.Bool(true)
				args = os.Args[3:]
			}
		}

		cache := &cache{
			cache:       make(map[string]*entry),
			predeclared: predeclared,
		}

		ch := make(chan string)
		for _, arg := range args {
			go func(buildfile string) {
				globals, err := cache.Load(buildfile)
				if err != nil {
					log.Fatal(err)
				}
				ch <- fmt.Sprintf("%s = %s", buildfile, globals)
			}(arg)
		}

		for _, arg := range args {
			println("Globals[" + arg + "]: " + <-ch)
		}
	}
}
