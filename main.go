package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

var (
	errAuthFailed = errors.New("Auth failed, key error")
	logger        *MultiLogger
	key           string
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
	tlsListenAndServe(ctx, ":443", config, nil)
	stdinHandleRegister("exit", exit, nil)
	stdinListenerAndServe(ctx, nil)
	select {}
}

func exit(param ...string) {
	os.Exit(0)
}

func authIn(conn net.Conn, param []byte) error {
	if string(param) == key {
		conn.Write([]byte("ok\x00"))
		return nil
	}
	conn.Write([]byte("failed\x00"))
	return errAuthFailed
}
