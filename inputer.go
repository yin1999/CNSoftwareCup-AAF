package main

import (
	"bufio"
	"context"
	"os"
)

var handlerMapping map[string]func(param []string)

func init() {
	handlerMapping = make(map[string]func(param []string))
}

func stdinListener(input chan string) {
	reader := bufio.NewReader(os.Stdin)
	for {
		res, err := reader.ReadString('\n')
		if err != nil {
			logger.Println(err)
			return
		}
		input <- res
	}
}

func inputHandler(ctx context.Context, input chan string) {
	for {
		select {
		case <-ctx.Done():
			return
		case in := <-input:
			cmd, param := cmdSplit(in)
			if f, ok := handlerMapping[cmd]; ok {
				f(param)
			}
		}
	}
}

func cmdSplit(in string) (command string, param []string) {
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
				command = string(buf[:bufIndex+1])
			} else {
				param = append(param, string(buf[:bufIndex+1]))
			}
			bufIndex = 0
		} else {
			buf[bufIndex] = in[i]
			bufIndex++
			flag = false
		}
	}
	return
}
