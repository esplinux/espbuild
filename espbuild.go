package main

import "go.starlark.net/starlark"

import "bufio"
import "fmt"
import "log"
import "os"
import "os/exec"
import "strings"
import "sync"

func toString(arg starlark.Value) (string) {
  argStr,isString := starlark.AsString( arg )
  if isString {
    return argStr
  } else {
    return ""
  }
}

func shell(arg string) {
  //out, err := exec.Command("/bin/sh", "-xec", arg).CombinedOutput()
  //fmt.Printf("%s", out)

  //if err != nil {
  //  log.Fatal(err)
  //}

  var wg sync.WaitGroup

  cmd := exec.Command("/bin/sh", "-xec", arg)

  stdout, err := cmd.StdoutPipe()
  if err != nil {
    log.Fatal(err)
  }
  stderr, err := cmd.StderrPipe()
  if err != nil {
    log.Fatal(err)
  }

  if err := cmd.Start(); err != nil {
    log.Fatal(err)
  }

  outch := make(chan string, 10)

  scannerStdout := bufio.NewScanner(stdout)
  scannerStdout.Split(bufio.ScanLines)
  wg.Add(1)
  go func() {
    for scannerStdout.Scan() {
        text := scannerStdout.Text()
        if strings.TrimSpace(text) != "" {
            outch <- text
        }
    }
    wg.Done()
  }()
  scannerStderr := bufio.NewScanner(stderr)
  scannerStderr.Split(bufio.ScanLines)
  wg.Add(1)
  go func() {
    for scannerStderr.Scan() {
        text := scannerStderr.Text()
        if strings.TrimSpace(text) != "" {
            outch <- text
        }
    }
    wg.Done()
  }()

  go func() {
    wg.Wait()
    close(outch)
  }()

  for t := range outch {
    fmt.Println(t)
  }

  if err := cmd.Wait(); err != nil {
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
  if len(os.Args) < 2 {
    log.Fatal("You must supply the config file to build")
  }

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
    "install": starlark.NewBuiltin("install", shellBuiltIn),
    "package": starlark.NewBuiltin("package", shellBuiltIn),
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

