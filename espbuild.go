package main

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/containers/buildah"
	"github.com/containers/storage/pkg/unshare"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go/build"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// DEBUG enables debug logging
const DEBUG = false

// builderCache is a cache of container name to builder references
var containerCache = make(map[string]*container)

func getContainerName(name string) string {
	return strings.Split(name, ": ")[1]
}

func getContainer(b *starlark.Builtin) *container {
	return containerCache[getContainerName(b.Name())]
}

func containerAdd(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	debug("invoking container.add " + thread.Name + " on " + b.Name())

	var file string
	dest := "/"
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "file", &file, "dest?", &dest); err != nil {
		return nil, err
	}

	c := getContainer(b)
	return starlark.None, c.add(file, dest)
}

func containerRun(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	debug("invoking container.run " + thread.Name + " on " + b.Name())

	var cmd = &starlark.List{}
	var quiet bool
	var env = &starlark.Dict{}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "cmd", &cmd, "quiet?", &quiet, "env?", &env); err != nil {
		return starlark.None, err
	}

	c := getContainer(b)
	return c.run(cmd, quiet, env)
}

func containerSetCmd(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	debug("invoking container.setCmd " + thread.Name + " on " + b.Name())

	var cmd = &starlark.List{}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "cmd", &cmd); err != nil {
		return starlark.None, err
	}

	c := getContainer(b)
	return starlark.None, c.setCmd(cmd)
}

func containerCommit(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	debug("invoking container.commit " + thread.Name + " on " + b.Name())

	var name string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "name", &name); err != nil {
		return starlark.None, err
	}

	c := getContainer(b)
	return c.commit(name)
}

func containerBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	debug("invoking container " + thread.Name)

	var from string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "from", &from); err != nil {
		return starlark.None, err
	}

	c, err := NewContainer(from)
	if err != nil {
		return nil, err
	}

	sd := starlark.StringDict{
		"add":    starlark.NewBuiltin("container.add: "+c.name(), containerAdd),
		"run":    starlark.NewBuiltin("container.run: "+c.name(), containerRun),
		"setCmd": starlark.NewBuiltin("container.setCmd: "+c.name(), containerSetCmd),
		"commit": starlark.NewBuiltin("container.commit: "+c.name(), containerCommit),
	}

	containerCache[c.name()] = c
	return starlarkstruct.FromStringDict(starlark.String("container: "+c.name()), sd), nil
}

func fetchBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	debug("invoking fetch " + thread.Name)

	buildfile, err := filepath.Abs(thread.Name)
	if err != nil {
		return starlark.None, err
	}
	curdir := filepath.Dir(buildfile)

	var http, git, branch, file string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "http?", &http, "file?", &file, "git?", &git, "branch?", &branch); err != nil {
		return starlark.None, err
	}

	if http != "" {
		if file != "" {
			return getHttpFile(http, curdir, file)
		}
		return getHttpSource(http, curdir)
	} else if git != "" {
		return getGit(git, branch, curdir)
	} else {
		return starlark.None, errors.New("source only supports git and http")
	}
}

func findBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	debug("invoking find " + thread.Name)

	var glob string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "glob", &glob); err != nil {
		return nil, err
	}

	globMatches, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}

	var results []starlark.Value
	for _, globMatch := range globMatches {
		err = filepath.Walk(globMatch, func(path string, info os.FileInfo, err error) error {
			results = append(results, starlark.String(path))
			return nil
		})
		if err != nil {
			return starlark.None, err
		}
	}

	return starlark.NewList(results), nil
}

func isDirBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	debug("invoking isDir " + thread.Name)

	var file string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "file", &file); err != nil {
		return nil, err
	}

	// ensure the file actually exists before trying to tar it
	fileInfo, err := os.Stat(file)
	if err != nil {
		return starlark.None, fmt.Errorf("unable to isDir file - %v", err.Error())
	}

	return starlark.Bool(fileInfo.Mode().IsDir()), nil
}

func lstatBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	debug("invoking lstat " + thread.Name)

	var file string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "file", &file); err != nil {
		return nil, err
	}

	// ensure the file actually exists before trying to tar it
	fileInfo, err := os.Lstat(file)
	if err != nil {
		return starlark.None, fmt.Errorf("unable to lstat file - %v", err.Error())
	}

	name := fileInfo.Name()
	if fileInfo.Mode()&os.ModeSymlink != 0 { // check if Symlink bit set
		name, err = os.Readlink(file) // Set name to link
		return starlark.String(name), err
	}

	return starlark.String(""), nil
}

func matchBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	debug("invoking match " + thread.Name)

	var regex, s string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "regex", &regex, "s", &s); err != nil {
		return nil, err
	}
	matched, err := regexp.MatchString(regex, s)
	return starlark.Bool(matched), err
}

func pathBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	debug("invoking path " + thread.Name)

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

func shellBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	debug("invoking shell " + thread.Name)

	var command string
	var quiet bool
	var env = &starlark.Dict{}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "command", &command, "quiet?", &quiet, "env?", &env); err != nil {
		return starlark.None, err
	}
	return shell(command, quiet, env)
}

func tarBuiltIn(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	debug("invoking tar " + thread.Name)

	var name, baseDir string
	var files = &starlark.List{}
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "name", &name, "basedir", &baseDir, "files", &files); err != nil {
		return starlark.None, err
	}
	return Tar(name, baseDir, files)
}

func getPredeclared() starlark.StringDict {
	predeclared := starlark.StringDict{
		"container": starlark.NewBuiltin("container", containerBuiltIn),
		"fetch":     starlark.NewBuiltin("fetch", fetchBuiltIn),
		"find":      starlark.NewBuiltin("find", findBuiltIn),
		"isDir":     starlark.NewBuiltin("isDir", isDirBuiltIn),
		"lstat":     starlark.NewBuiltin("lstat", lstatBuiltIn),
		"match":     starlark.NewBuiltin("match", matchBuiltIn),
		"path":      starlark.NewBuiltin("path", pathBuiltIn),
		"shell":     starlark.NewBuiltin("shell", shellBuiltIn),
		"struct":    starlark.NewBuiltin("struct", starlarkstruct.Make),
		"tar":       starlark.NewBuiltin("tar", tarBuiltIn),
		"NPROC":     starlark.String(strconv.Itoa(runtime.NumCPU())),
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

func contains(sp *[]string, s string) bool {
	for _, ss := range *sp {
		if ss == s {
			return true
		}
	}

	return false
}

func preProcess(buildFiles *[]string, buildFile string) {
	file, err := os.Open(buildFile)
	if err != nil {
		log.Fatal(err)
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Bit of a hack of a parser but does the job for now
		line := scanner.Text()
		if strings.ContainsRune(line, '#') {
			line = strings.Split(line, "#")[0]
		}

		if strings.Contains(line, "load(\"") {
			load := strings.Split(strings.Split(line, "load(\"")[1], "\"")[0]
			if !contains(buildFiles, load) {
				*buildFiles = append(*buildFiles, load)
				preProcess(buildFiles, load)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		go fatal(err)
	}

	if err := file.Close(); err != nil {
		fatal(err)
	}
}

func getBuiltInsPath() string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		gopath = build.Default.GOPATH
	}

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

	return builtins
}

// todo: emolitor refactor to use flags
func main() {
	if buildah.InitReexec() {
		return
	}
	unshare.MaybeReexecUsingUserNamespace(false)

	/*
		container, err := NewContainer("scratch")
		if err != nil {
			fatal(err)
		}

		if err := container.addRoot("/home/emolitor/Development/esplinux/packages/toybox-HEAD-0.tgz"); err != nil {
			fatal(err)
		}
		if err := container.addRoot("/home/emolitor/Development/esplinux/packages/dash-0.5.10.2-0.tgz"); err != nil {
			fatal(err)
		}

		container.setCmd("sh")

		imageId, err := container.commit()
		if err != nil {
			fatal(err)
		}

		println("imageID: " + imageId)
	*/
	// Begin actual main
	if len(os.Args) < 2 {
		fmt.Println("Usage:")
		fmt.Println("\tespbuild package.esp")
	} else {
		builtinsPath := getBuiltInsPath()
		predeclared := getPredeclared()
		globals, err := starlark.ExecFile(&starlark.Thread{Name: "BuiltIns"}, builtinsPath, nil, predeclared)
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

		var buildFiles []string
		for _, arg := range args {
			buildFiles = append(buildFiles, arg)
			preProcess(&buildFiles, arg)
		}

		cache := &cache{
			cache:       make(map[string]*entry),
			predeclared: predeclared,
		}

		ch := make(chan string)
		for _, buildFile := range buildFiles {
			go func(buildfile string) {
				globals, err := cache.Load(buildfile)
				if err != nil {
					log.Fatal(err)
				}
				ch <- fmt.Sprintf("%s = %s", buildfile, globals)
			}(buildFile)
		}

		for range buildFiles {
			<-ch
		}
	}
}
