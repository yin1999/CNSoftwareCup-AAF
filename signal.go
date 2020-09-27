package main

import (
	"context"
	"os"
	"os/signal"
)

var signalMapping map[os.Signal]func()

func init() {
	signalMapping = make(map[os.Signal]func())
}

func signalHandleRegister(s os.Signal, handler func(), mapping map[os.Signal]func()) {
	if mapping == nil {
		signalMapping[s] = handler
	} else {
		mapping[s] = handler
	}
}

func signalListenAndServe(ctx context.Context, mapping map[os.Signal]func()) {
	c := make(chan os.Signal, 1)
	if mapping == nil {
		mapping = signalMapping
	}
	for k := range mapping {
		signal.Notify(c, k)
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case s := <-c:
				go mapping[s]()
			}
		}
	}()
}
