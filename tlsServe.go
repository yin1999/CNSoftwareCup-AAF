package main

import (
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
	var buf []byte
	for {
		_, err := conn.Read(buf)
		fmt.Println(buf)
		if err != nil {
			return
		}
	}
}
