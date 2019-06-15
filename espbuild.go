package main

import "go.starlark.net/starlark"

import "bufio"
import "fmt"
import "log"
import "os"
import "os/exec"
import "strings"
import "sync"

var dependencyProg string

func toString(arg starlark.Value) string {
  argStr,isString := starlark.AsString( arg )
  if isString {
    return argStr
  } else {
    return ""
  }
}

func shell(arg string) {
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
  if args.Len() < 1 || len(kwargs) !=0 {
    log.Fatalf("%s-%s: requires a single string argument", thread.Name, b.Name())
  }

  shell( toString(args[0]) )

  return starlark.None, nil
}

func checkoutBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
  if args.Len() < 1 && len(kwargs) < 1 {
    log.Fatalf("%s-%s: requires one or more arguments", thread.Name, b.Name())
  }

  if args.Len() > 0 {
    shell( toString(args[0]) )
  }

  if len(kwargs) > 0 {
    command := toString(kwargs[0].Index(0))
    subCommand := ""
    if len(kwargs) > 1 {
      subCommand = toString(kwargs[1].Index(0))
    }

    if command == "url" {
      url := toString( kwargs[0].Index(1) )
      if _, err := os.Stat("/bin/bsdtar"); err == nil {
        shell("curl -L " + url + " | bsdtar -xf -")
      } else {
        shell("curl -LO " + url)
        shell("basename " + url + " | xargs tar xf")
      }
    } else if command == "git" {
      url := toString( kwargs[0].Index(1) )

      if subCommand == "branch" {
        branch := toString( kwargs[1].Index(1) )
        shell("git clone --branch " + branch + " --depth 1 " + url)
      } else {
        shell("git clone " + url)
      }
    } else {
      for index, element := range kwargs {
        for i:=0; i < element.Len(); i++ {
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
    depCommand := "echo " + toString(args.Index(i)) + " | xargs -n1 " + dependencyProg
    shell(depCommand)
  }

  return starlark.None, nil
}

func main() {

  if _, err := os.Stat("/etc/esp-release"); err == nil {
    // esp based system
    dependencyProg = "/bin/esp add"

  } else if os.IsNotExist(err) {
    // Assume apk based system
    dependencyProg = "/sbin/apk add"

  } else {
    log.Fatal("Unable to determine underlying system")
  }

  if len(os.Args) < 2 {
    log.Fatal("You must supply the config file to build")
  }

  script := os.Args[1]

  if len(os.Args) > 2 {
    if os.Args[2] == "--nodeps" {
      println("Dependencies disabled")
      dependencyProg = "echo"
    }
  }

  thread := &starlark.Thread{
	  Name: "espbuild",
  }

  predeclared := starlark.StringDict{
    "dependencies": starlark.NewBuiltin("dependencies", dependenciesBuiltIn),
    "pre": starlark.NewBuiltin("pre", shellBuiltIn),
    "checkout": starlark.NewBuiltin("checkout", checkoutBuiltIn),
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

