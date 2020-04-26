package main

import (
	"bytes"
	"context"
	"github.com/containers/buildah"
	is "github.com/containers/image/v5/storage"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage"
	"github.com/containers/storage/pkg/unshare"
	"go.starlark.net/starlark"
)

type container struct {
	builder *buildah.Builder
}

func getStore() (storage.Store, error) {
	buildStoreOptions, err := storage.DefaultStoreOptions(unshare.IsRootless(), unshare.GetRootlessUID())
	if err != nil {
		return nil, err
	}

	return storage.GetStore(buildStoreOptions)
}

// NewContainer creates a new container
func NewContainer(from string) (*container, error) {
	buildStore, err := getStore()
	if err != nil {
		fatal(err)
	}

	opts := buildah.BuilderOptions{
		FromImage:        from,
		Isolation:        buildah.IsolationChroot,
		CommonBuildOpts:  &buildah.CommonBuildOptions{},
		ConfigureNetwork: buildah.NetworkDefault,
		SystemContext:    &types.SystemContext{},
	}

	c := new(container)
	c.builder, err = buildah.NewBuilder(context.TODO(), buildStore, opts)

	return c, err
}

// add adds file to the dest director of the container
func (c *container) add(file string, dest string) error {
	return c.builder.Add(dest, true, buildah.AddAndCopyOptions{}, file)
}

// add adds file to the dest director of the container
func (c *container) run(command *starlark.List, quiet bool, env *starlark.Dict) (starlark.Value, error) {
	var cmd []string

	commandIter := command.Iterate()
	defer commandIter.Done()
	var commandK starlark.Value
	for commandIter.Next(&commandK) {
		s, _ := starlark.AsString(commandK)
		cmd = append(cmd, s)
	}

	nameSpaceOptions, err := buildah.DefaultNamespaceOptions()
	if err != nil {
		return starlark.None, err
	}

	var outBuf bytes.Buffer

	var envList []string
	envIter := env.Iterate()
	defer envIter.Done()
	var envK starlark.Value
	for envIter.Next(&envK) {
		v, _, err := env.Get(envK)
		if err != nil {
			return starlark.None, err
		}

		key, _ := starlark.AsString(envK)
		value, _ := starlark.AsString(v)

		envList = append(envList, key+"="+value)
	}

	runOptions := buildah.RunOptions{
		Hostname:  "localhost",
		Isolation: buildah.IsolationChroot,
		//Runtime:          "",
		Args: nil,
		//NoPivot:          false,
		//Mounts:           nil,
		Env: envList,
		//User:             "",
		//WorkingDir:       "",
		//Shell:            "",
		//Cmd:              nil,
		//Entrypoint:       nil,
		NamespaceOptions: nameSpaceOptions,
		ConfigureNetwork: buildah.NetworkDefault,
		//CNIPluginPath:    "",
		//CNIConfigDir:     "",
		Terminal: buildah.DefaultTerminal,
		//TerminalSize:     nil,
		//Stdin:            nil,
		Stdout: &outBuf,
		Stderr: &outBuf,
		Quiet:  quiet,
		//AddCapabilities:  nil,
		//DropCapabilities: nil,
		//Devices:          nil,
	}

	err = c.builder.Run(cmd, runOptions)

	return starlark.String(outBuf.String()), err
}

// setCmd provides the defaults for an executing container
func (c *container) setCmd(command *starlark.List) error {
	var commands []string

	iter := command.Iterate()
	defer iter.Done()
	var k starlark.Value
	for iter.Next(&k) {
		s, _ := starlark.AsString(k)
		commands = append(commands, s)
	}

	c.builder.SetCmd(commands)
	return nil
}

// commit commits the container and returns the imageId of the new image
func (c *container) commit(name string) (starlark.Value, error) {
	buildStore, err := getStore()
	if err != nil {
		return starlark.None, err
	}

	imageRef, err := is.Transport.ParseStoreReference(buildStore, name)
	if err != nil {
		return starlark.None, err
	}

	imageId, _, _, err := c.builder.Commit(context.TODO(), imageRef, buildah.CommitOptions{})
	return starlark.String(imageId), err
}

func (c *container) name() string {
	return c.builder.Container
}
