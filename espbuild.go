package main

import "go.starlark.net/starlark"

import "fmt"
import "log"
import "os"
import "os/exec"
import "strings"

const packageDir = "packages"
var dependencyProg string
var verbose = true

func toString(arg starlark.Value) string {
  argStr,isString := starlark.AsString( arg )
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
    _, err := proc.Wait()
    if err != nil {
      log.Fatal("shell: Error during wait ", err)
    }
  } else {
    log.Fatal("shell: Unable to run command ", err)
  }
}

/**
func shell(arg string) {
  var wg sync.WaitGroup

  cmd := exec.Command("/bin/sh", "-xec", arg)

  stdout, err := cmd.StdoutPipe()
  if err != nil {
    log.Fatal("Unable to get StdoutPipe ", err)
  }
  stderr, err := cmd.StderrPipe()
  if err != nil {
    log.Fatal("Unable to get StderrPipe ", err)
  }

  if err := cmd.Start(); err != nil {
    log.Fatal("Unable to start command ", err)
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
    log.Fatal("Error during wait", err)
  }
}**/

func shellBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
  if args.Len() < 1 || len(kwargs) !=0 {
    log.Fatalf("%s-%s: requires a single string argument", thread.Name, b.Name())
  }

  if verbose {
    fmt.Printf("shell(%s)\n", toString(args[0]) )
  }

  shell( toString(args[0]) )

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
    if verbose {
      fmt.Printf("dependencies(%s)\n", toString(args.Index(i)))
    }

    depCommand := "echo " + toString(args.Index(i)) + " | xargs -n1 " + dependencyProg
    shell(depCommand)
  }

  return starlark.None, nil
}

func packageBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
  if args.Len() > 1 || len(kwargs) != 0 {
    log.Fatalf("%s-%s: requires one or less arguments", thread.Name, b.Name())
  }

  name := os.Getenv("NAME")
  version := os.Getenv("VERSION")
  sourceDir := name

  if verbose {
    fmt.Printf("package(%s, %s, %s)\n", name, version, sourceDir)
  }

  if args.Len() > 0 {
    sourceDir = toString(args[0])
  }


  shell("mkdir -p " + packageDir)
  shell("bsdtar jcf " + packageDir + "/$NAME-$VERSION.tar.bz2 --strip-components=1 " + sourceDir)
  shell("echo " + name + "_VERSION=" + version + " > " + packageDir + "/$NAME.manifest")
  shell("echo " + name + "_FILE=$NAME-$VERSION.tar.gz >> " + packageDir + "/$NAME.manifest")
  shell("echo " + name + "_SHA1=$(sha1sum " + packageDir +"/$NAME-$VERSION.tar.bz2 | cut -d' ' -f1) >> " + packageDir + "/$NAME.manifest")
  shell("echo " + name + "_URL=\"https://github.com/esplinux-core/$NAME/releases/download\" >> " + packageDir + "/$NAME.manifest")

  return starlark.None, nil
}

func subPackageBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
  if args.Len() < 1 || len(kwargs) != 0 {
    log.Fatalf("%s-%s: requires at least a single string argument", thread.Name, b.Name())
  }

  name := toString(args[0])
  sourceDir := name

  if verbose {
    fmt.Printf("subPackage(%s, %s)\n", name, sourceDir)
  }


  if args.Len() > 1 {
    sourceDir = toString(args[1])
  }

  shell("mkdir -p " + packageDir)
  shell("bsdtar jcf " + packageDir + "/" + name + "-$VERSION.tar.bz2 --strip-components=1 " + sourceDir)
  shell("echo " + name + "_FILE=" + name + "-$VERSION.tar.gz >> " + packageDir + "/$NAME.manifest")
  shell("echo " + name + "_SHA1=$(sha1sum " + packageDir +"/" + name + "-$VERSION.tar.bz2 | cut -d' ' -f1) >> " + packageDir + "/$NAME.manifest")
  shell("echo " + name + "_BASE=$NAME >> " + packageDir + "/$NAME.manifest")

  return starlark.None, nil
}

func envBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
  if args.Len() < 1 || len(kwargs) !=0 {
    log.Fatalf("%s-%s: requires a single string argument", thread.Name, b.Name())
  }

  if verbose {
    fmt.Printf("env(%s,%s)\n", strings.ToUpper(b.Name()), toString(args[0]))
  }

  setEnv(strings.ToUpper(b.Name()), toString(args[0]))

  return starlark.None, nil
}

func main() {
  fmt.Printf("ESPBuild\n")

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
    fmt.Printf("Dependency program: %s\n", dependencyProg)
  }

  if len(os.Args) < 2 {
    log.Fatal("You must supply the config file to build")
  }

  script := os.Args[1]

  if verbose {
    fmt.Printf("script: %s\n", script)
  }

  if len(os.Args) > 2 {
    if os.Args[2] == "--verbose" {
      verbose = true
    }
  }

  if verbose {
    fmt.Printf("Create thread...\n")
  }

  thread := &starlark.Thread{
	  Name: "espbuild",
  }

  fmt.Printf("Create predecls...\n")

  predeclared := starlark.StringDict{
    "name": starlark.NewBuiltin("name", envBuiltIn),
    "version": starlark.NewBuiltin("version", envBuiltIn),
    "dependencies": starlark.NewBuiltin("dependencies", dependenciesBuiltIn),
    "pre": starlark.NewBuiltin("pre", shellBuiltIn),
    "checkout": starlark.NewBuiltin("checkout", checkoutBuiltIn),
    "patch": starlark.NewBuiltin("patch", shellBuiltIn),
    "config": starlark.NewBuiltin("config", shellBuiltIn),
    "build": starlark.NewBuiltin("build", shellBuiltIn),
    "install": starlark.NewBuiltin("install", shellBuiltIn),
    "package": starlark.NewBuiltin("package", packageBuiltIn),
    "subpackage": starlark.NewBuiltin("package", subPackageBuiltIn),
    "post": starlark.NewBuiltin("post", shellBuiltIn),
    "shell": starlark.NewBuiltin("shell", shellBuiltIn),
  }

  fmt.Printf("Run...\n")

  _, err := starlark.ExecFile(thread, script, nil, predeclared)

  if err != nil {
    if evalErr, ok := err.(*starlark.EvalError); ok {
      log.Fatal(evalErr.Backtrace())
    }
    log.Fatal(err)
  }
}

