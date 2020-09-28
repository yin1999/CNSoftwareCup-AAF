package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"net"
	"sync"
)

type tlsHandlerFunc func(conn net.Conn, param []byte) error
type sessionID string

var (
	errCloseConnect   = errors.New("Please close the connection")
	listenerClosed    = false
	m                 = sync.Mutex{}
	tlsHandlerMapping = make(map[string]tlsHandlerFunc)
	sessionMapping    = make(map[sessionID]net.Conn)
)

func tlsConnectHandleRegister(cmd string, f tlsHandlerFunc, mapping map[string]tlsHandlerFunc) {
	if mapping == nil {
		mapping = tlsHandlerMapping
	}
	mapping[cmd] = f
}

func tlsListenAndServe(ctx context.Context, laddr string, cfg *tls.Config, mapping map[string]tlsHandlerFunc) {
	if mapping == nil {
		mapping = tlsHandlerMapping
	}
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
				go tlsConnectHandler(conn, mapping)
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
				continue
			}
			connChannel <- conn
		}
	}()
	return connChannel
}

func tlsConnectHandler(conn net.Conn, mapping map[string]tlsHandlerFunc) {
	sess := sessionIDGen()
	logger.Printf("New connect: %v.\n", sess)
	sessionMapping[sess] = conn
	defer sessionClose(sess)
	r := bufio.NewReader(conn)
	if f, ok := mapping["auth"]; ok {
		err := f(conn, nil)
		if err != nil {
			logger.Println(err)
			return
		}
	}
	for {
		msg, err := r.ReadBytes(0)
		if err != nil {
			logger.Println(err)
			return
		}
		cmd, data := dataSplit(msg)
		err = mapping[cmd](conn, data)
		switch err {
		case errCloseConnect:
			return
		case nil:
			break
		default:
			logger.Printf("session: %s, %v.\n", sess, err)
		}
	}
}

func dataSplit(in []byte) (cmd string, data []byte) {
	i := 0
	for i = range in {
		if in[i] == ':' {
			return string(in[:i]), in[i+1 : len(in)-1]
		}
	}
	return string(in[:i+1]), nil
}

func sessionIDGen() sessionID {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return sessionID(base64.URLEncoding.EncodeToString(b))
}

func sessionClose(sess sessionID) {
	logger.Printf("sess: %s closed.\n", sess)
	sessionMapping[sess].Close()
	delete(sessionMapping, sess)
}
