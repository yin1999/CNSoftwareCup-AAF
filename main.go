package main

import (
	"context"
	"crypto/tls"
	"os"
	"time"
)

var (
	logger *MultiLogger
)

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
	signalHandleRegister(os.Interrupt, cancel, nil)
	signalHandleRegister(os.Kill, cancel, nil)
	signalListenAndServe(ctx, nil)
	tlsListenAndServe(ctx, ":443", config)
}
