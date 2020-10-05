package main

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/moby/moby/client"
)

// PathType 路径类型
type PathType byte

const (
	execDocker = "/usr/bin/docker"
)

const (
	notExist PathType = iota
	directory
	file
)

var (
	imageMapping = map[fileType]string{
		python2: "registry-vpc.cn-shanghai.aliyuncs.com/yin199909/centos_7:origin",
		python3: "registry-vpc.cn-shanghai.aliyuncs.com/yin199909/centos_7:python3.6",
		golang:  "registry-vpc.cn-shanghai.aliyuncs.com/yin199909/centos_7:origin",
	}
	errNotSupport = errors.New("Path type not support")
)

func newProcess(ctx context.Context, p programInfo, argv string, dbList []dbInfo) (string, error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		return "", err
	}
	sess := sessionIDGen(16)
	cmd := []string{"sh", "-c"}
	switch p.file {
	case python2:
		cmd = append(cmd, fmt.Sprintf("pip2 install -r requirements.txt && python2 main.py %s %s", sess, argv))
	case python3:
		cmd = append(cmd, fmt.Sprintf("pip3 install -r requirements.txt && python3 main.py %s %s", sess, argv))
	case golang:
		cmd = append(cmd, fmt.Sprintf("./main %s %s", sess, argv))
	}
	body, err := cli.ContainerCreate(ctx, &container.Config{
		Image:      imageMapping[p.file],
		Cmd:        cmd,
		WorkingDir: "/app",
	}, nil, nil, "")
	if err != nil {
		cli.Close()
		return "", err
	}
	if err = copyToContainer(ctx, cli, body.ID, "/app/", p.dir); err != nil {
		cli.ContainerRemove(context.Background(), body.ID, types.ContainerRemoveOptions{Force: true})
		cli.Close()
		return "", err
	}
	containerSessToID[sess] = body.ID
	dbListMapping[body.ID] = dbList
	if err = cli.ContainerStart(ctx, body.ID, types.ContainerStartOptions{}); err != nil {
		cli.ContainerRemove(context.Background(), body.ID, types.ContainerRemoveOptions{Force: true})
		cli.Close()
		delete(containerSessToID, sess)
		dbInfoRemove(body.ID)
		return "", err
	}
	go containerListenAndServe(ctx, cli, body.ID, sess)
	return body.ID, nil
}

// TODO
func containerListenAndServe(ctx context.Context, cli *client.Client, containerID string, sess sessionID) {
	returnCode, err := cli.ContainerWait(ctx, containerID)
	logger.Printf("Container: %s return %d.\n", containerID, returnCode)
	if err != nil {
		logger.Printf("Exit with error: %s.\n", err.Error())
	}
	mqLock.Lock() // 互斥锁上锁
	mqSend([]byte(fmt.Sprintf("stoped:%s:%d\x00", containerID, returnCode)))
	if v, ok := processMapping[containerID]; ok {
		if v.immediate == false {
			if data, ok := dataMapping[containerID]; ok {
				lengthBuf := int32Encoder(int32(len(data)))
				mqSend([]byte(fmt.Sprintf("data:%s\x00", containerID)))
				mqSend(lengthBuf)
				mqSend(data)
			}
		}
	}
	mqLock.Unlock() // 互斥锁解锁
	cli.ContainerRemove(context.Background(), containerID, types.ContainerRemoveOptions{Force: true})
	dataRead(containerID)
	dbInfoRemove(containerID)
	processCancel(containerID)
	cli.Close()
	delete(containerSessToID, sess)
}

func copyToContainer(ctx context.Context, cli *client.Client, containerID, dst, src string) error {
	buf := new(bytes.Buffer)
	err := Tar(src, buf)
	if err != nil {
		return err
	}
	return cli.CopyToContainer(ctx, containerID, dst, buf, types.CopyToContainerOptions{AllowOverwriteDirWithFile: true})
}

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

// Tar 打包文件或目录
func Tar(src string, dst *bytes.Buffer) error {
	length := len(src)
	flag := false
	switch pathStat(src) {
	case notExist:
		return os.ErrNotExist
	case directory: // 去掉root目录
		if src[length-1] != filepath.Separator {
			length++
		}
		flag = true
	case file:
		dir := filepath.Dir(src)
		length = len(dir)
		if length == 1 && dir[0] == '.' {
			length = 0
		}
	}
	tw := tar.NewWriter(dst)
	defer tw.Close()
	return filepath.Walk(src, func(fileName string, fi os.FileInfo, err error) error {
		if flag || err != nil {
			flag = false
			return err
		}
		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		hdr.Name = fileName[length:] // 去掉根目录

		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !fi.Mode().IsRegular() { // not file: dir ...
			return nil
		}
		fileReader, err := os.Open(fileName)
		if err != nil {
			return err
		}
		defer fileReader.Close()
		_, err = io.Copy(tw, fileReader)
		return err
	})
}
