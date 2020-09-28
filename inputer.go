package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
)

var handlerMapping = make(map[string]func(param ...string))

func stdinHandleRegister(cmd string, f func(param ...string), mapping map[string]func(param ...string)) {
	if mapping == nil {
		mapping = handlerMapping
	}
	mapping[cmd] = f
}

func stdinListenerAndServe(ctx context.Context, mapping map[string]func(param ...string)) {
	if mapping == nil {
		mapping = handlerMapping
	}
	input := make(chan []byte, 1)
	reader := bufio.NewReader(os.Stdin)
	go func() {
		for {
			res, err := reader.ReadBytes('\n')
			if err != nil {
				logger.Println(err)
				return
			}
			l := len(res)
			if l == 1 {
				continue
			}
			if res[l-2] == '\r' {
				input <- res[:l-2]
			} else {
				input <- res[:l-1]
			}
		}
	}()
	go inputHandler(ctx, input, mapping)
}

func inputHandler(ctx context.Context, input chan []byte, mapping map[string]func(param ...string)) {
	for {
		select {
		case <-ctx.Done():
			return
		case in := <-input:
			cmd, param := cmdSplit(in)
			if f, ok := mapping[cmd]; ok {
				f(param...)
			} else {
				fmt.Printf("Unknown command: %s\n", cmd)
			}
		}
	}
}

func cmdSplit(in []byte) (command string, param []string) {
	fmt.Println(len(in))
	buf := make([]byte, len(in))
	bufIndex := 0
	cmd := true
	flag := true
	for i := range in {
		if in[i] == ' ' {
			if flag {
				continue
			}
			flag = true
			if cmd {
				command = string(buf[:bufIndex])
				cmd = false
			} else {
				param = append(param, string(buf[:bufIndex]))
			}
			bufIndex = 0
		} else {
			buf[bufIndex] = in[i]
			bufIndex++
			flag = false
		}
	}
	if cmd {
		command = string(buf[:bufIndex])
	} else {
		param = append(param, string(buf[:bufIndex]))
	}
	return
}
