//go:build js && wasm

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/crhntr/window"
	"github.com/crhntr/window/dom"

	"github.com/crhntr/mockscript/mockscript"
)

func main() {
	c := make(chan struct{}, 0)

	rtnBtn := window.Document().QuerySelector(`#execution button[name="return"]`)
	fn := js.FuncOf(returnButton)
	defer fn.Release()
	options := dom.AddEventListenerOptions{}
	rtnBtn.AddEventListener("click", fn, options, false)
	defer rtnBtn.RemoveEventListener("click", fn, options, false)

	exec := window.NewEventSource("/exec", true)
	exec.On("ExecutionResult", func(event dom.MessageEvent) {
		var message mockscript.ExecutionResult
		d := event.Data()
		_ = json.Unmarshal([]byte(d), &message)
		argsDiv := window.Document().QuerySelector(`#execution #arguments`)
		exec.Close()
		argsDiv.Closest(`#execution`).
			AppendChild(window.Document().
				CreateTextNode(fmt.Sprintf("exit %d", message.ExitCode)))
	})

	exec.On("InvokedExecution", func(event dom.MessageEvent) {
		var message mockscript.InvokedExecution
		argsDiv := window.Document().QuerySelector(`#execution #arguments`)
		argsDiv.SetTextContent(strings.Join(message.Args, " "))
	})
	<-c
}

func returnButton(_ js.Value, args []js.Value) any {
	input := dom.HTMLElement(dom.InputEvent(args[0]).Target()).
		Closest(`#execution`).
		QuerySelector(`input[name="exit-code"]`).(dom.HTMLElement)
	value, err := strconv.Atoi(js.Value(input).Get("value").String())
	if err != nil {
		window.ConsoleLog(err.Error())
		return nil
	}
	body, err := json.Marshal(mockscript.ExecutionResult{
		ExitCode: value,
	})
	if err != nil {
		window.ConsoleLog(err.Error())
		return nil
	}
	req, err := http.NewRequest(http.MethodPost, "/return", bytes.NewReader(body))
	if err != nil {
		window.ConsoleLog(err.Error())
		return nil
	}
	go func() {
		_, err = http.DefaultClient.Do(req)
		if err != nil {
			window.ConsoleLog(err.Error())
		}
	}()
	return nil
}
