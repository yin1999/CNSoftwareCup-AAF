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
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	bufferSlice          = 2048
	ctxRoot              context.Context
	ctxRootCancel        context.CancelFunc
	errAuthFailed        = errors.New("Auth failed, key error")
	errTypeErr           = errors.New("Unknown type")
	errEOF               = errors.New("Error EOF")
	errNoID              = errors.New("ID not existed")
	errTransferErr       = errors.New("Transfer err, got wrong data")
	errNoMapping         = errors.New("No value with this key")
	mqLock               = sync.Mutex{}
	logger               *MultiLogger
	key                  string
	statusOK             = []byte("ok\x00")
	statusErr            = []byte("error\x00")
	statusTypeErr        = []byte("typeErr\x00")
	storePath            = "program"
	programMapping       = make(map[programIndex]programInfo)
	pwd                  string
	tcpForDocker         = make(map[string]tcpHandlerFunc)
	addressToContainerID = make(map[string]string)
	dbListMapping        = make(map[string][]dbInfo)
	processMapping       = make(map[string]processInfo)
	containerSessToID    = make(map[sessionID]string)
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
	tcpConnectHandleRegister("getFile", getFile, nil)
	tcpConnectHandleRegister("listen", statusListenRegister, nil)
	tcpConnectHandleRegister("start", execStart, nil)
	tcpConnectHandleRegister("stop", execStop, nil)
	tcpConnectHandleRegister("disconnect", disconnectForListener, nil)
	tcpConnectHandleRegister("auth", authForDocker, tcpForDocker)
	tcpConnectHandleRegister("disconnect", disconnectForDocker, tcpForDocker)
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
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	data, _ = readBytes(0, r)
	if string(data) == key {
		conn.Write(statusOK)
		conn.SetReadDeadline(time.Time{})
		return nil
	}
	conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	conn.Write(statusErr)
	return errAuthFailed
}

func authForDocker(conn net.Conn, data []byte) error {
	r := bufio.NewReader(conn)
	sess, _ := readString(0, r)
	if v, ok := containerSessToID[sessionID(sess)]; ok {
		addressToContainerID[conn.RemoteAddr().String()] = v
		conn.Write(statusOK)
		return nil
	}
	conn.Write(statusErr)
	return errAuthFailed
}

func disconnectForDocker(conn net.Conn, data []byte) error {
	delete(addressToContainerID, conn.RemoteAddr().String())
	return nil
}

func disconnectForListener(conn net.Conn, data []byte) error {
	listenerLock.Lock()
	if connListener != nil && connListener.RemoteAddr() == conn.RemoteAddr() {
		connListener = nil
		runtime.SetFinalizer(conn, nil)
	}
	listenerLock.Unlock()
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
	logger.Printf("New Listener: %s.\n", conn.RemoteAddr().String())
	return nil
}

func statusSend(conn net.Conn, data []byte) {
	if connListener != nil {
		length := len(data)
		i := bufferSlice
		for ; i <= length; i += bufferSlice {
			connListener.Write(data[i-bufferSlice : i])
			time.Sleep(40 * time.Millisecond)
		}
		if length%bufferSlice != 0 {
			connListener.Write(data[i-bufferSlice:])
		}
	}
}

func execStart(conn net.Conn, data []byte) error {
	if len(data) < 1 {
		conn.Write(statusErr)
		return errTypeErr
	}
	id := programIndex(data) // programID
	var p programInfo
	var ok bool
	if p, ok = programMapping[id]; !ok {
		conn.Write(statusErr)
		return errNoID
	}
	conn.Write(statusOK) // response
	// Get Argv
	r := bufio.NewReader(conn)
	argv, err := readString(0, r)
	if err != nil {
		conn.Write(statusErr)
		return err
	}
	conn.Write(statusOK) // response

	// transfer dbInfo
	// number of database(1 byte) +
	// db Type; db Address; db database; db userName; db password + "\x00" ....(repeat)
	num := make([]byte, 1)
	conn.Read(num)
	dbList := make([]dbInfo, int(num[0]))
	flag := false
	for i := byte(0); i < num[0]; i++ {
		info, err := readString(0, r)
		l := strings.Split(info, ";")
		if err != nil || len(l) != 5 {
			flag = true
			continue
		}
		dbList[i].Type = l[0]
		dbList[i].Addr = l[1]
		dbList[i].Database = l[2]
		dbList[i].UserName = l[3]
		dbList[i].Password = l[4]
	}
	if flag {
		conn.Write(statusErr)
		return errTransferErr
	}
	containerID, err := newProcess(p.ctx, p, argv, dbList)
	if err != nil {
		conn.Write(statusErr)
		return err
	}
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
		f, _ := os.Open("source/driver.py")
		w, _ := os.OpenFile(path+"/driver.py", os.O_CREATE|os.O_WRONLY, 0644)
		io.Copy(w, f)
		f.Close()
		w.Close()
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
	conn.Write(statusOK)
	data = make([]byte, 4)
	conn.Read(data)
	length := binary.BigEndian.Uint32(data[:4])
	if _, err = io.CopyN(file, conn, int64(length)); err != nil {
		file.Close()
		os.RemoveAll(path)
		conn.Write(statusErr)
		return err
	}
	file.Close()
	if err = builder(s); err != nil {
		os.RemoveAll(path)
		conn.Write(statusErr)
		return err
	}
	conn.Write(statusOK)
	conn.Write([]byte(id + "\x00"))
	s.ctx, s.cancel = context.WithCancel(ctxRoot)
	programMapping[programIndex(id)] = s
	s.cfgStore()
	return err
}

func getFile(conn net.Conn, data []byte) error {
	id := programIndex(data)
	v, ok := programMapping[id]
	if !ok {
		conn.Write(statusErr)
		return errNoMapping
	}
	var fileName string
	switch v.file {
	case python2, python3:
		fileName = v.dir + "/main.py"
	case golang:
		fileName = v.dir + "main.go"
	}
	buf, err := ioutil.ReadFile(fileName)
	if err != nil {
		conn.Write(statusErr)
		return err
	}
	length := int32Encoder(int32(len(buf)))
	conn.Write(length)
	conn.Write(buf)
	return nil
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
	if id, ok := addressToContainerID[conn.RemoteAddr().String()]; ok {
		return id
	}
	return ""
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
	if id == "" {
		conn.Write(statusErr)
		return errNoID
	}
	conn.Write(statusOK)
	if v, ok := processMapping[id]; ok {
		data = make([]byte, 4)
		conn.Read(data)
		length := int(binary.BigEndian.Uint32(data))
		raw := make([]byte, length)
		i := bufferSlice
		for ; i <= length; i += bufferSlice {
			conn.Read(raw[i-bufferSlice : i])
		}
		if length%bufferSlice != 0 {
			conn.Read(raw[i-bufferSlice:])
		}
		if v.immediate {
			mqLock.Lock()
			mqSend([]byte(fmt.Sprintf("data:%s\x00", id)))
			mqSend(data)
			mqSend(raw)
			mqLock.Unlock()
		} else {
			dataStore(id, raw)
		}
		conn.Write(statusOK)
		return nil
	}
	conn.Close()
	return errors.New("Stop this process")
}

func processCancel(containerID string) {
	if v, ok := processMapping[containerID]; ok {
		v.cancel()
		delete(processMapping, containerID)
	}
}
