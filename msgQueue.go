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
		listenerLock.Unlock()
		mqHead = mqHead.next
		mqMutex.Unlock()
	}
}
