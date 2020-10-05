package main

import (
	"net"
	"sync"
)

type msgNode struct {
	data []byte
	next *msgNode
}

var (
	connListener      net.Conn
	mqHead            *msgNode
	mqTail            *msgNode
	mqMutex           = sync.Mutex{}
	pushServiceLocked = false
	pushLock          = make(chan struct{}, 1)
	listenerLock      = sync.Mutex{}
)

func init() {
	go mqPush() // start push service
}

func mqSend(data []byte) {
	node := &msgNode{
		data: data,
		next: nil,
	}
	mqMutex.Lock()
	if mqHead == nil {
		mqHead = node
		pushLock <- struct{}{}
	} else {
		mqTail.next = node
	}
	mqTail = node
	mqMutex.Unlock()
}

// mqPush plz call this function with go routine
func mqPush() {
	for {
		mqMutex.Lock()
		listenerLock.Lock()
		if mqHead == nil || connListener == nil {
			pushServiceLocked = true
			listenerLock.Unlock()
			mqMutex.Unlock()
			select {
			case <-ctxRoot.Done():
				return
			case <-pushLock:
				continue
			}
		}
		statusSend(connListener, mqHead.data)
		mqHead = mqHead.next
		listenerLock.Unlock()
		mqMutex.Unlock()
	}
}

func int32Encoder(num int32) []byte {
	buf := make([]byte, 4)
	for i := 3; num != 0; i-- {
		buf[i] = byte(num & 0xFF)
		num >>= 8
	}
	return buf
}
