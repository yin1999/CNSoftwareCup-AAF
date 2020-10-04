package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type fileType int
type programIndex string
type programInfo struct {
	dir       string
	file      fileType
	immediate bool
	ctx       context.Context
	cancel    context.CancelFunc
}

type processInfo struct {
	cancel    context.CancelFunc
	immediate bool
}

const (
	python2 fileType = iota
	python3
	golang
)

var (
	ctxRoot           context.Context
	ctxRootCancel     context.CancelFunc
	errAuthFailed     = errors.New("Auth failed, key error")
	errTypeErr        = errors.New("Unknown type")
	errEOF            = errors.New("Error EOF")
	errNoID           = errors.New("ID not existed")
	errTransferErr    = errors.New("Transfer err, got wrong data")
	errNoMapping      = errors.New("No value with this key")
	logger            *MultiLogger
	key               string
	statusOK          = []byte("ok\x00")
	statusErr         = []byte("error\x00")
	statusTypeErr     = []byte("typeErr\x00")
	storePath         = "program"
	programMapping    = make(map[programIndex]programInfo)
	pwd               string
	tcpForDocker      = make(map[string]tcpHandlerFunc)
	portToContainerID = make(map[string]string)
	dbListMapping     = make(map[string][]dbInfo)
	processMapping    = make(map[string]processInfo)
)

func init() {
	f, err := os.Open("login.key")
	if err != nil {
		os.Exit(-1)
	}
	fmt.Fscanln(f, &key)
	f.Close()
	ctxRoot, ctxRootCancel = context.WithCancel(context.Background())
	logger = NewMultiLogger(30*24*time.Hour, "log")
	IDReader(programMapping)
	pwd = filepath.Dir(os.Args[0]) + "/"
}

func main() {
	logger.Println("Starting...")
	cert, err := tls.LoadX509KeyPair("CA/xx.hhuiot.xyz.pem", "CA/xx.hhuiot.xyz.key")
	if err != nil {
		logger.Println(err)
		return
	}

	config := &tls.Config{Certificates: []tls.Certificate{cert}}
	defer ctxRootCancel()
	signalHandleRegister(os.Interrupt, ctxRootCancel, nil)
	signalHandleRegister(os.Kill, ctxRootCancel, nil)
	signalListenAndServe(ctxRoot, nil)
	tcpConnectHandleRegister("auth", authIn, nil)
	tcpConnectHandleRegister("fileTransfer", fileReceiver, nil)
	tcpConnectHandleRegister("removeFile", fileRemover, nil)
	tcpConnectHandleRegister("listen", statusListenRegister, nil)
	tcpConnectHandleRegister("start", execStart, nil)
	tcpConnectHandleRegister("stop", execStop, nil)
	tcpConnectHandleRegister("auth", authForDocker, tcpForDocker)
	tcpConnectHandleRegister("dbList", dbInfoGet, tcpForDocker)
	tcpConnectHandleRegister("send", dataSend, tcpForDocker)
	tcpListenAndServe(ctxRoot, ":443", config, nil) // exposed port
	tcpListenAndServe(ctxRoot, ":2076", nil, tcpForDocker)
	stdinHandleRegister("exit", exit, nil)
	stdinHandleRegister("listSession", listSession, nil)
	stdinListenerAndServe(ctxRoot, nil)
	select {}
}

func exit(param ...string) {
	os.Exit(0)
}

func listSession(param ...string) {
	fmt.Print("Session\t\tRemoteAddr\n")
	for k, v := range sessionMapping {
		fmt.Printf("%s\t\t%s", k, v.RemoteAddr().String())
	}
}

func authIn(conn net.Conn, data []byte) error {
	r := bufio.NewReader(conn)
	data, _ = readBytes(0, r)
	if string(data) == key {
		conn.Write(statusOK)
		return nil
	}
	conn.Write(statusErr)
	return errAuthFailed
}

func authForDocker(conn net.Conn, data []byte) error {
	return nil
}

func statusListenRegister(conn net.Conn, data []byte) error {
	listenerLock.Lock()
	if connListener != nil {
		connListener.Close()
		runtime.SetFinalizer(connListener, nil)
	}
	connListener = conn
	if pushServiceLocked {
		pushLock <- struct{}{}
	}
	listenerLock.Unlock()
	runtime.SetFinalizer(connListener, (net.Conn).Close)
	conn.Write(statusOK)
	return nil
}

func statusSend(conn net.Conn, data []byte) {
	if connListener != nil {
		connListener.SetWriteDeadline(time.Now().Add(3 * time.Second))
		if _, err := connListener.Write(data); err != nil {
			connListener.Close()
			runtime.SetFinalizer(connListener, nil)
		}
	}
}

func execStart(conn net.Conn, data []byte) error {
	if len(data) < 1 {
		conn.Write(statusErr)
		return errTypeErr
	}
	id := programIndex(data)
	var p programInfo
	var ok bool
	if p, ok = programMapping[id]; !ok {
		conn.Write(statusErr)
		return errNoID
	}
	conn.Write(statusOK) // response
	r := bufio.NewReader(conn)
	argv, err := readString(0, r)
	if err != nil {
		conn.Write(statusErr)
		return err
	}
	conn.Write(statusOK) // response

	// transfer dbInfo
	// number of database(1 byte) +
	// db Type; db Address; db userName; db password; ....(repeat) + "\x00"
	num := make([]byte, 1)
	conn.Read(num)
	dbList := make([]dbInfo, int(num[0]))
	flag := true
	for i := byte(0); i < num[0]; i++ {
		dbList[i].dbType, err = readString(';', r)
		if err != nil {
			flag = false
		}
		dbList[i].dbAddr, err = readString(';', r)
		if err != nil {
			flag = false
		}
		dbList[i].dbUserName, err = readString(';', r)
		if err != nil {
			flag = false
		}
		dbList[i].dbPassword, err = readString(';', r)
		if err != nil {
			flag = false
		}
	}
	conn.Read(num)
	if num[0] != 0 || flag == false { // end symbol
		conn.Write(statusErr)
		return errTransferErr
	}
	ctx, cancel := context.WithCancel(p.ctx)
	containerID, err := newProcess(ctx, p, argv, dbList)
	if err != nil {
		conn.Write(statusErr)
		cancel()
		return err
	}
	dbListMapping[containerID] = dbList
	processMapping[containerID] = processInfo{cancel: cancel, immediate: p.immediate}
	conn.Write(statusOK) // response + containerID + "\x00"
	conn.Write([]byte(containerID + "\x00"))
	return err
}

func execStop(conn net.Conn, data []byte) error {
	containerID := string(data)
	if v, ok := processMapping[containerID]; ok {
		v.cancel()
		delete(processMapping, containerID)
		conn.Write(statusOK)
		return nil
	}
	conn.Write(statusErr)
	return errNoID
}

// fileReceiver
// cmd format: "fileTransfer" + ":" + type(lower bit: filetype, higher bit result type) + "\x00" + fileSize(bytes) + "\x00"
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
	s.dir = path
	switch data[0] & 0x7F {
	case 1:
		s.file = python2
		fileName += "py"
	case 2:
		s.file = python3
		fileName += "py"
	case 3:
		s.file = golang
		fileName += "go"
	default:
		conn.Write(statusTypeErr)
		return errTypeErr
	}
	if (data[0] & 0x80) == 0x80 {
		s.immediate = true
	} else {
		s.immediate = false
	}
	file, err := os.OpenFile(path+"/"+fileName, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		conn.Write(statusErr)
		return err
	}
	data = make([]byte, 5)
	conn.Read(data)
	if data[4] != 0 {
		file.Close()
		conn.Write(statusErr)
		os.RemoveAll(path)
		return errNotSupport
	}
	conn.Write(statusOK) // response
	length := binary.BigEndian.Uint32(data[:4])
	if _, err = io.CopyN(file, conn, int64(length)); err != nil {
		conn.Write(statusErr)
		file.Close()
		os.RemoveAll(path)
		return err
	}
	file.Close()
	if err = builder(s); err != nil {
		conn.Write(statusErr)
		os.RemoveAll(path)
		return err
	}
	conn.Write([]byte(id + "\x00"))
	s.ctx, s.cancel = context.WithCancel(ctxRoot)
	programMapping[programIndex(id)] = s
	s.cfgStore()
	return err
}

// fileRemover
// cmd format: "removeID"+ ":" + ID
// return: statusErr, ID not existed; statusOK, remove this file successfully
func fileRemover(conn net.Conn, data []byte) error {
	id := programIndex(data)
	if v, ok := programMapping[id]; ok {
		delete(programMapping, id)
		os.RemoveAll(v.dir)
		v.cancel()
		conn.Write(statusOK)
		return nil
	}
	conn.Write(statusErr)
	return errNoID
}

func connToID(conn net.Conn) (containerID string) {
	port := strings.Split(conn.RemoteAddr().String(), ":")[1]
	if id, ok := portToContainerID[port]; ok {
		return id
	}
	return ""
}

func addIDToMapping(containerID, port string) {
	portToContainerID[port] = containerID
}

func dbInfoGet(conn net.Conn, data []byte) error {
	id := connToID(conn)
	if id == "" {
		return errNoID
	}
	if v, ok := dbListMapping[id]; ok {
		data, err := json.Marshal(v)
		if err != nil {
			conn.Write(statusErr)
			return err
		}
		conn.Write(append(data, 0))
		return err
	}
	return errNoMapping
}

func dbInfoRemove(containerID string) {
	delete(dbListMapping, containerID)
}

func dataSend(conn net.Conn, data []byte) error {
	id := connToID(conn)
	if v, ok := processMapping[id]; ok {
		if v.immediate {
			cmd := []byte(id + ":")
			cmd = append(cmd, data...)
			mqSend(append(cmd, 0))
		} else {
			dataStore(id, data)
		}
		conn.Write(statusOK)
		return nil
	}
	conn.Close()
	return errors.New("Exit this program")
}

func processCancel(containerID string) {
	if v, ok := processMapping[containerID]; ok {
		v.cancel()
		delete(processMapping, containerID)
	}
}
