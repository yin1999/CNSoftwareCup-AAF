package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"log"
	"net"
	"sync"
)

type tcpHandlerFunc func(conn net.Conn, param []byte) error
type sessionID string

var (
	errCloseConnect   = errors.New("Please close the connection")
	listenerClosed    = false
	m                 = sync.Mutex{}
	tlsHandlerMapping = make(map[string]tcpHandlerFunc)
	sessionMapping    = make(map[sessionID]net.Conn)
)

func tcpConnectHandleRegister(cmd string, f tcpHandlerFunc, mapping map[string]tcpHandlerFunc) {
	if mapping == nil {
		mapping = tlsHandlerMapping
	}
	mapping[cmd] = f
}

func tcpListenAndServe(ctx context.Context, laddr string, cfg *tls.Config, mapping map[string]tcpHandlerFunc) {
	if mapping == nil {
		mapping = tlsHandlerMapping
	}
	var ln net.Listener
	var err error
	if cfg != nil {
		ln, err = tls.Listen("tcp", laddr, cfg)
	} else {
		ln, err = net.Listen("tcp", laddr)
	}

	if err != nil {
		logger.Println(err)
		return
	}
	ch := tcpListener(ln)
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
				go tcpConnectHandler(conn, mapping)
			}
		}
	}()
}

func tcpListener(ln net.Listener) <-chan net.Conn {
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

func tcpConnectHandler(conn net.Conn, mapping map[string]tcpHandlerFunc) {
	sess := sessionIDGen(12)
	logger.Printf("New connect from %s: %s.\n", conn.RemoteAddr().String(), sess)
	sessionMapping[sess] = conn
	defer sessionClose(sess)
	if f, ok := mapping["disconnect"]; ok {
		defer f(conn, nil)
	}
	r := bufio.NewReader(conn)
	if f, ok := mapping["auth"]; ok {
		if err := f(conn, nil); err != nil {
			logger.Println(err)
			return
		}
	}
	for {
		msg, err := readBytes(0, r)
		if err != nil {
			logger.Println(err)
			return
		}
		cmd, data := dataSplit(msg)
		if f, ok := mapping[cmd]; ok {
			log.Printf("session: %s, cmd: %s.\n", sess, cmd)
			err = f(conn, data)
			switch err {
			case errCloseConnect:
				return
			case nil:
				break
			default:
				logger.Printf("session: %s, %v.\n", sess, err)
			}
		} else {
			logger.Printf("Session: %s, unknow cmd: %s.\n", sess, cmd)
		}
	}
}

func dataSplit(in []byte) (cmd string, data []byte) {
	i := 0
	for i = range in {
		if in[i] == ':' {
			return string(in[:i]), in[i+1:]
		}
	}
	return string(in), nil
}

func sessionIDGen(length int) sessionID {
	b := make([]byte, length)
	for {
		if _, err := rand.Read(b); err != nil {
			return ""
		}
		id := sessionID(base64.RawURLEncoding.EncodeToString(b))
		if _, ok := sessionMapping[id]; !ok {
			return id
		}
	}
}

func sessionClose(sess sessionID) {
	sessionMapping[sess].Close()
	delete(sessionMapping, sess)
	logger.Printf("Session: %s closed.\n", sess)
}

func readBytes(delimer byte, r *bufio.Reader) ([]byte, error) {
	data, err := r.ReadBytes(delimer)
	if err != nil {
		return nil, err
	}
	return data[:len(data)-1], err
}

func readString(delimer byte, r *bufio.Reader) (string, error) {
	data, err := readBytes(delimer, r)
	return string(data), err
}
