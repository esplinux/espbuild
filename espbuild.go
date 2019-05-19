package main

import "go.starlark.net/starlark"

import "fmt"
import "log"
import "os"
import "os/exec"

func toString(arg starlark.Value) (string) {
  argStr,isString := starlark.AsString( arg )
  if isString {
    return argStr
  } else {
    return ""
  }
}

func shell(arg string) {
  out, err := exec.Command("/bin/sh", "-xec", arg).CombinedOutput()
  fmt.Printf("%s", out)

  if err != nil {
    log.Fatal(err)
  }
}

func shellBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
  if args.Len() < 1 {
    log.Fatalf("%s: requires one or more arguments", b.Name);
  }

  shell( toString(args[0]) )

  return starlark.None, nil
}

func dependenciesBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

  if args.Len() < 1 {
	  log.Fatalf("%s: requires one or more arguments", b.Name);
  }

  depCommand := "apk add"

  for i := 0; i < args.Len(); i++ {
    depCommand = depCommand + " " + toString( args.Index(i) )
  }

  shell(depCommand)

  return starlark.None, nil
}

func main() {
  script := os.Args[1]
  thread := &starlark.Thread{
	  Name: "espbuild",
  }

  predeclared := starlark.StringDict{
    "dependencies": starlark.NewBuiltin("dependencies", dependenciesBuiltIn),
    "pre": starlark.NewBuiltin("pre", shellBuiltIn),
    "checkout": starlark.NewBuiltin("checkout", shellBuiltIn),
    "patch": starlark.NewBuiltin("patch", shellBuiltIn),
    "config": starlark.NewBuiltin("config", shellBuiltIn),
    "build": starlark.NewBuiltin("build", shellBuiltIn),
    "post": starlark.NewBuiltin("post", shellBuiltIn),
  }

  _, err := starlark.ExecFile(thread, script, nil, predeclared)

  if err != nil {
    if evalErr, ok := err.(*starlark.EvalError); ok {
      log.Fatal(evalErr.Backtrace())
    }
    log.Fatal(err)
  }
}

