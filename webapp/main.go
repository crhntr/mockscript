//go:build js && wasm

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
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

	argsDiv := window.Document().QuerySelector(`#execution #arguments`)

	exec := window.NewEventSource("/exec", true)

	on(exec, func(event mockscript.ExecutionResult) {
		defer exec.Close()
		argsDiv.Closest(`#execution`).
			AppendChild(window.Document().
				CreateTextNode(fmt.Sprintf("exit %d", event.ExitCode)))
	})
	on(exec, func(event mockscript.InvokedExecution) {
		argsDiv.SetTextContent(strings.Join(event.Args, " "))
	})
	<-c
}

func on[T any](src dom.EventSource, handler func(event T)) {
	var zero T
	src.On(reflect.TypeOf(zero).Name(), func(event dom.MessageEvent) {
		var value T
		_ = json.Unmarshal([]byte(event.Data()), &value)
		handler(value)
	})
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
