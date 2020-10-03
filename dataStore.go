package main

import (
	"context"
	"io/ioutil"
	"log"
	"os"
)

var (
	dataMapping = make(map[string][]byte)
	py2File     = []byte{0}
	py3File     = []byte{1}
	goFile      = []byte{2}
)

func dataStore(containerID string, data []byte) {
	if v, ok := dataMapping[containerID]; ok {
		dataMapping[containerID] = append(v, data...)
	} else {
		dataMapping[containerID] = data
	}
}

// Read Data and remove the mapping
func dataRead(containerID string) []byte {
	if v, ok := dataMapping[containerID]; ok {
		delete(dataMapping, containerID)
		return v
	}
	return nil
}

func (p programInfo) cfgStore() error {
	f, err := os.OpenFile(p.dir+"/status", os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	switch p.file {
	case python2:
		f.Write(py2File)
	case python3:
		f.Write(py3File)
	case golang:
		f.Write(goFile)
	}
	if p.immediate {
		f.Write([]byte{1})
	} else {
		f.Write([]byte{0})
	}
	f.Close()
	return err
}

func (p *programInfo) cfgLoader(dir string) error {
	f, err := os.Open(dir + "/status")
	if err != nil {
		return err
	}
	p.dir = dir
	buf := make([]byte, 2)
	f.Read(buf)
	f.Close()
	switch buf[0] {
	case 0:
		p.file = python2
	case 1:
		p.file = python3
	case 2:
		p.file = golang
	}
	if buf[1] == 1 {
		p.immediate = true
	}
	p.ctx, p.cancel = context.WithCancel(ctxRoot)
	return err
}

// IDReader 初始化时获取programInfo
func IDReader(mapping map[programIndex]programInfo) {
	if mapping == nil {
		return
	}
	files, err := ioutil.ReadDir(storePath)
	if err != nil {
		logger.Println(err)
	}
	p := &programInfo{}
	for i := range files {
		name := files[i].Name()
		err = p.cfgLoader(storePath + "/" + name)
		if err != nil {
			log.Println(err)
		} else {
			mapping[programIndex(name)] = *p
		}
	}
}
