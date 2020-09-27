package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
)

var (
	listenerClosed = false
	m              = sync.Mutex{}
)

func tlsListenAndServe(ctx context.Context, laddr string, cfg *tls.Config) {
	ln, err := tls.Listen("tcp", laddr, cfg)
	if err != nil {
		logger.Println(err)
		ctx.Done()
		return
	}
	ch := tlsListener(ln)
	go func() {
		for {
			select {
			case <-ctx.Done():
				m.Lock()
				ln.Close()
				listenerClosed = true
				m.Unlock()
				return
			case conn := <-ch:
				go connectHandler(conn)
			}
		}
	}()
}

func tlsListener(ln net.Listener) <-chan net.Conn {
	connChannel := make(chan net.Conn, 1)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				m.Lock()
				if listenerClosed {
					m.Unlock()
					return
				}
				m.Unlock()
				logger.Println(err)
			}
			go connectHandler(conn)
		}
	}()
	return connChannel
}

func connectHandler(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		msg, err := r.ReadBytes(0)
		if err != nil {
			logger.Println(err)
			return
		}
		msg = msg[:len(msg)-1]
		fmt.Println(len(msg))
		fmt.Println(msg)
	}
}
