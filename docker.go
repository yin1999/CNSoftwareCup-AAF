package main

import (
	"context"
	"errors"
	"io/ioutil"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/moby/moby/client"
)

const (
	execDocker = "/usr/bin/docker"
)

var (
	imageMapping = map[fileType]string{
		python2: "registry-vpc.cn-shanghai.aliyuncs.com/yin199909/centos_7:origin",
		python3: "registry-vpc.cn-shanghai.aliyuncs.com/yin199909/centos_7:python3.6",
		golang:  "registry-vpc.cn-shanghai.aliyuncs.com/yin199909/centos_7:origin",
	}
	errNotSupport = errors.New("Path type not support")
)

type containerID string

// PathType 路径类型
type PathType byte

func newProcess(ctx context.Context, p programInfo) error {
	cli, err := client.NewEnvClient()
	if err != nil {
		return err
	}
	cmd := []string{"sh", "-c"}
	switch p.file {
	case python2:
		cmd = append(cmd, "cd /app && pip2 install -r requirements.txt && python2 main.py")
	case python3:
		cmd = append(cmd, "cd /app && pip3 install -r requirements.txt && python3 main.py")
	case golang:
		cmd = append(cmd, "cd /app && ./main")
	}
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: imageMapping[p.file],
		Cmd:   []string{"/bin/bash"},
	}, nil, nil, "")

	if err != nil {
		return err
	}

	if err = copyToContainer(ctx, cli, resp.ID, "/app/", p.dir); err != nil {
		return err
	}

	if err = cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		return err
	}
	return nil
}

func copyToContainer(ctx context.Context, cli *client.Client, containerID, dst, src string) error {
	switch pathStat(src) {
	case notExist:
		return os.ErrNotExist
	case directory:
		if dst[len(dst)-1] != '/' {
			dst += "/"
		}
		if src[len(src)-1] != '/' {
			src += "/"
		}
		files, err := ioutil.ReadDir(src)
		if err != nil {
			return err
		}
		for i := range files {
			fileName := files[i].Name()
			err = copyToContainer(ctx, cli, containerID, dst+fileName, src+fileName)
			if err != nil {
				return err
			}
		}
		return err
	case file:
		f, err := os.Open(src)
		if err != nil {
			return err
		}
		err = cli.CopyToContainer(ctx, containerID, dst, f, types.CopyToContainerOptions{})
		f.Close()
		return err
	default:
		return errNotSupport
	}
}

const (
	notExist PathType = iota
	directory
	file
)

// pathStat 判断所给路径类型
// return: directory: 文件夹, file: 文件, notExist: 不存在
func pathStat(path string) PathType {
	s, err := os.Stat(path)
	if err != nil {
		return notExist
	}
	if s.IsDir() {
		return directory
	}
	return file
}
