package main

import (
	"go.starlark.net/starlark"
)

import "fmt"
import "log"
import "os"
import "os/exec"
import "path"
import "strings"

const packageDir = "packages"

var dependencyProg string
var verbose = false

func toString(arg starlark.Value) string {
	argStr, isString := starlark.AsString(arg)
	if isString {
		return argStr
	} else {
		return ""
	}
}

func setEnv(name string, value string) {
	err := os.Setenv(name, value)
	if err != nil {
		log.Fatal("Unable to set environment", err)
	}
}

func Start(args ...string) (p *os.Process, err error) {
	if args[0], err = exec.LookPath(args[0]); err == nil {
		var procAttr os.ProcAttr
		procAttr.Files = []*os.File{os.Stdin, os.Stdout, os.Stderr}
		p, err := os.StartProcess(args[0], args, &procAttr)
		if err == nil {
			return p, nil
		}
	}
	return nil, err
}

func shell(args ...string) {
	args = append([]string{"/bin/sh", "-xec"}, args...)

	if proc, err := Start(args...); err == nil {
		processState, err := proc.Wait()
		if err != nil {
			log.Fatal("shell: Error during wait ", err)
		} else if processState.ExitCode() != 0 {
			log.Fatal("shell: Exited with non zero status")
		}
	} else {
		log.Fatal("shell: Unable to run command ", err)
	}
}

func shellBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if args.Len() < 1 || len(kwargs) != 0 {
		log.Fatalf("%s-%s: requires a single string argument", thread.Name, b.Name())
	}

	if verbose {
		fmt.Printf("shell(%s)\n", toString(args[0]))
	}

	shell(toString(args[0]))

	return starlark.None, nil
}

func checkoutBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if args.Len() < 1 && len(kwargs) < 1 {
		log.Fatalf("%s-%s: requires one or more arguments", thread.Name, b.Name())
	}

	if verbose {
		fmt.Printf("checkout()\n")
	}

	if args.Len() > 0 {
		shell(toString(args[0]))
	}

	if len(kwargs) > 0 {
		command := toString(kwargs[0].Index(0))
		subCommand := ""
		if len(kwargs) > 1 {
			subCommand = toString(kwargs[1].Index(0))
		}

		if command == "url" {
			url := toString(kwargs[0].Index(1))

			file := path.Base(url)

			err := DownloadFile(file, url)
			if err != nil {
				panic(err)
			}

			extractTar(".", file)
		} else if command == "git" {
			url := toString(kwargs[0].Index(1))

			if subCommand == "branch" {
				branch := toString(kwargs[1].Index(1))
				err := cloneBranch("src", url, branch)
				if err != nil {
					log.Fatal(err)
				}
			} else if subCommand == "tag" {
				tag := toString(kwargs[1].Index(1))
				err := cloneTag("src", url, tag)
				if err != nil {
					log.Fatal(err)
				}
			} else if subCommand == "commit" {
				commit := toString(kwargs[1].Index(1))
				err := cloneCheckout("src", url, commit)
				if err != nil {
					log.Fatal(err)
				}
			} else {
				err := clone("src", url)
				if err != nil {
					log.Fatal(err)
				}
			}
		} else {
			for index, element := range kwargs {
				for i := 0; i < element.Len(); i++ {
					fmt.Printf("UNKNOWN CHECKOUT COMMAND %v-%v:%v:%s\n", command, index, i, element.Index(i))
				}
			}
			log.Fatal("UNKNWON CHECKOUT COMMAND!!!!")
		}
	}

	return starlark.None, nil
}

func dependenciesBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	if args.Len() < 1 || len(kwargs) != 0 {
		log.Fatalf("%s-%s: requires one or more arguments", thread.Name, b.Name())
	}

	for i := 0; i < args.Len(); i++ {
		shell(dependencyProg + " " + toString(args.Index(i)))
	}

	return starlark.None, nil
}

func packageBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if args.Len() > 1 || len(kwargs) != 0 {
		log.Fatalf("%s-%s: requires one or less arguments", thread.Name, b.Name())
	}

	name := os.Getenv("NAME")
	version := os.Getenv("VERSION")
	sourceDir := "build-" + name

	if args.Len() > 0 {
		sourceDir = toString(args[0])
	}

	if verbose {
		fmt.Printf("package(%s, %s, %s)\n", name, version, sourceDir)
	}

	shell("mkdir -p " + packageDir)

	//shell("tar jcf " + packageDir + "/$NAME-$VERSION.tar.bz2 -C " + sourceDir + " .")
	err := createTar(sourceDir, packageDir+"/"+name+"-"+version+".tar.bz2")
	if err != nil {
		log.Fatal(err)
	}

	shell("echo " + name + "_VERSION=" + version + " > " + packageDir + "/$NAME.manifest")
	shell("echo " + name + "_FILE=$NAME-$VERSION.tar.bz2 >> " + packageDir + "/$NAME.manifest")
	shell("echo " + name + "_SHA256=$(sha256sum " + packageDir + "/$NAME-$VERSION.tar.bz2 | cut -d' ' -f1) >> " + packageDir + "/$NAME.manifest")
	shell("echo " + name + "_URL=\"/esplinux/core/releases/download\" >> " + packageDir + "/$NAME.manifest")

	return starlark.None, nil
}

func subPackageBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if args.Len() < 1 || len(kwargs) != 0 {
		log.Fatalf("%s-%s: requires at least a single string argument", thread.Name, b.Name())
	}

	name := toString(args[0])
	version := os.Getenv("VERSION")
	sourceDir := "build-" + toString(args[0])

	if args.Len() > 1 {
		sourceDir = toString(args[1])
	}

	if verbose {
		fmt.Printf("subPackage(%s, %s)\n", name, sourceDir)
	}

	shell("mkdir -p " + packageDir)

	//shell("tar jcf " + packageDir + "/" + name + "-$VERSION.tar.bz2 -C " + sourceDir + " .")
	err := createTar(sourceDir, packageDir+"/"+name+"-"+version+".tar.bz2")
	if err != nil {
		log.Fatal(err)
	}

	shell("echo " + name + "_FILE=" + name + "-$VERSION.tar.bz2 >> " + packageDir + "/$NAME.manifest")
	shell("echo " + name + "_SHA256=$(sha256sum " + packageDir + "/" + name + "-$VERSION.tar.bz2 | cut -d' ' -f1) >> " + packageDir + "/$NAME.manifest")
	shell("echo " + name + "_BASE=$NAME >> " + packageDir + "/$NAME.manifest")

	return starlark.None, nil
}

func envBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if args.Len() < 1 || len(kwargs) != 0 {
		log.Fatalf("%s-%s: requires a single string argument", thread.Name, b.Name())
	}

	if verbose {
		fmt.Printf("env(%s,%s)\n", strings.ToUpper(b.Name()), toString(args[0]))
	}

	setEnv(strings.ToUpper(b.Name()), toString(args[0]))

	return starlark.None, nil
}

func main() {

	if len(os.Args) < 2 {
		log.Fatal("You must supply the config file to build")
	}

	script := os.Args[1]

	if len(os.Args) > 2 {
		if os.Args[2] == "--verbose" {
			verbose = true
		}
	}

	if _, err := os.Stat("/etc/esp-release"); err == nil {
		// esp based system
		dependencyProg = "/bin/esp add"
	} else if os.IsNotExist(err) {
		// Assume apk based system
		dependencyProg = "/sbin/apk add"
	} else {
		log.Fatal("Unable to determine underlying system")
	}

	if verbose {
		fmt.Printf("Dependency command: %s\n", dependencyProg)
	}

	thread := &starlark.Thread{
		Name: "espbuild",
	}

	predeclared := starlark.StringDict{
		"name":         starlark.NewBuiltin("name", envBuiltIn),
		"version":      starlark.NewBuiltin("version", envBuiltIn),
		"dependencies": starlark.NewBuiltin("dependencies", dependenciesBuiltIn),
		"pre":          starlark.NewBuiltin("pre", shellBuiltIn),
		"checkout":     starlark.NewBuiltin("checkout", checkoutBuiltIn),
		"patch":        starlark.NewBuiltin("patch", shellBuiltIn),
		"config":       starlark.NewBuiltin("config", shellBuiltIn),
		"build":        starlark.NewBuiltin("build", shellBuiltIn),
		"install":      starlark.NewBuiltin("install", shellBuiltIn),
		"package":      starlark.NewBuiltin("package", packageBuiltIn),
		"subpackage":   starlark.NewBuiltin("package", subPackageBuiltIn),
		"post":         starlark.NewBuiltin("post", shellBuiltIn),
		"shell":        starlark.NewBuiltin("shell", shellBuiltIn),
	}

	_, err := starlark.ExecFile(thread, script, nil, predeclared)

	if err != nil {
		if evalErr, ok := err.(*starlark.EvalError); ok {
			log.Fatal(evalErr.Backtrace())
		}
		log.Fatal(err)
	}
}
