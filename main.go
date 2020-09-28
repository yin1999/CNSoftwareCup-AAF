package main

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

type fileType int

type programIndex string
type programInfo struct {
	dir  string
	file fileType
}

const (
	python fileType = iota
	golang
)

const (
	maxBufferLength = 4 << 20 // 4MB buffer
)

var (
	errAuthFailed  = errors.New("Auth failed, key error")
	errTypeErr     = errors.New("Unknown type")
	errEOF         = errors.New("Error EOF")
	errNoID        = errors.New("ID not existed")
	logger         *MultiLogger
	key            string
	statusOK       = []byte("ok\x00")
	statusErr      = []byte("error\x00")
	statusTypeErr  = []byte("typeErr\x00")
	storePath      = "program"
	programMapping = make(map[programIndex]programInfo)
)

func init() {
	f, err := os.Open("login.key")
	if err != nil {
		os.Exit(-1)
	}
	fmt.Fscanln(f, &key)
}

func main() {
	logger = NewMultiLogger(30*24*time.Hour, "log")
	logger.Println("Starting...")
	cert, err := tls.LoadX509KeyPair("CA/xx.hhuiot.xyz.pem", "CA/xx.hhuiot.xyz.key")
	if err != nil {
		logger.Println(err)
		return
	}

	config := &tls.Config{Certificates: []tls.Certificate{cert}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signalHandleRegister(os.Interrupt, cancel, nil)
	signalHandleRegister(os.Kill, cancel, nil)
	signalListenAndServe(ctx, nil)
	tlsConnectHandleRegister("auth", authIn, nil)
	tlsConnectHandleRegister("fileTransfer", fileReceiver, nil)
	tlsConnectHandleRegister("removeID", fileRemover, nil)
	tlsListenAndServe(ctx, ":443", config, nil)
	stdinHandleRegister("exit", exit, nil)
	stdinListenerAndServe(ctx, nil)
	select {}
}

func exit(param ...string) {
	os.Exit(0)
}

func listSession(param ...string) {
	// length := len(param)
	fmt.Print("Session\t\tRemoteAddr\n")
	// if length == 0 {
	for k, v := range sessionMapping {
		fmt.Printf("%s\t\t%s", k, v.RemoteAddr().String())
	}
	// }
}

func authIn(conn net.Conn, data []byte) error {
	if string(data) == key {
		conn.Write(statusOK)
		return nil
	}
	conn.Write(statusErr)
	return errAuthFailed
}

// fileReceiver
// cmd format: "fileTransfer" + ":" + fileType + fileSize(bytes) + "\x00"
// conn return: status
// if got statusOK, then transfer the file, if got No statusMsg, it means programID to the file
func fileReceiver(conn net.Conn, data []byte) error {
	id := fmt.Sprint(time.Now().Unix())
	path := fmt.Sprintf("%s/%s", storePath, id)
	err := os.MkdirAll(path, 0755)
	if err != nil {
		conn.Write(statusErr)
		return err
	}
	fileName := "main."
	s := programInfo{}
	switch data[0] {
	case 0:
		s.file = python
		fileName += "py"
	case 1:
		s.file = golang
		fileName += "go"
	default:
		conn.Write(statusTypeErr)
		return errTypeErr
	}
	file, err := os.OpenFile(path+"/"+fileName, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		conn.Write(statusErr)
		return err
	}
	conn.Write(statusOK)
	length := binary.BigEndian.Uint32(data[1:5]) + 1
	if length > maxBufferLength {
		buf := make([]byte, length)
		for length > maxBufferLength {
			conn.Read(buf)
			file.Write(buf)
			length -= maxBufferLength
		}
	}
	buf := make([]byte, length)
	conn.Read(buf)
	if buf[length-1] != 0 {
		conn.Write(statusErr)
		file.Close()
		os.RemoveAll(path)
		return errEOF
	}
	file.Write(buf[:length-1])
	conn.Write([]byte(id + "\x00"))
	file.Close()
	programMapping[programIndex(id)] = s
	return err
}

// fileRemover
// cmd format: "removeID" + ID
// return: statusErr, ID not existed; statusOK, remove this file successfully
func fileRemover(conn net.Conn, data []byte) error {
	id := programIndex(data)
	if _, ok := programMapping[id]; !ok {
		conn.Write(statusErr)
		return errNoID
	}
	delete(programMapping, id)
	conn.Write(statusOK)
	return nil
}
