package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"

	"github.com/crhntr/sse"
	"github.com/julienschmidt/httprouter"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/crhntr/mockscript/mockscript"
)

func main() {
	fileName := os.Args[1]

	fileContent, err := os.ReadFile(fileName)
	if err != nil {
		panic(err)
	}

	r := bytes.NewReader(fileContent)
	script, err := syntax.NewParser().Parse(r, filepath.Base(fileName))
	if err != nil {
		panic(err)
	}

	var (
		calls       = make(chan []string)
		returns     = make(chan int)
		scriptError = make(chan error)
	)

	go func() {
		defer close(calls)
		defer close(scriptError)
		runner, err := interp.New(
			interp.StdIO(nil, io.Discard, io.Discard),
			interp.ExecHandler(func(ctx context.Context, args []string) error {
				calls <- args
				ret := <-returns
				log.Println(args, " => ", ret)
				if ret != 0 {
					return interp.NewExitStatus(uint8(ret))
				}
				return nil
			}),
		)
		if err != nil {
			return
		}
		ctx := context.Background()
		err = runner.Run(ctx, script)
		if err != nil {
			scriptError <- err
		}
	}()

	mux := httprouter.New()
	mux.Handler(http.MethodPost, "/return", http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		requestBuffer, err := io.ReadAll(io.LimitReader(req.Body, 1024))
		if err != nil {
			http.Error(res, "failed to read body", http.StatusBadRequest)
			return
		}
		var data mockscript.ExecutionResult
		if err := json.Unmarshal(requestBuffer, &data); err != nil {
			http.Error(res, "failed to parse body", http.StatusBadRequest)
			return
		}
		returns <- data.ExitCode
		res.WriteHeader(http.StatusAccepted)
	}))
	mux.Handler(http.MethodGet, "/exec", http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		eventSource := NewEventSource(res)
		sse.SetHeaders(res)
		res.WriteHeader(http.StatusOK)

		for i := 2; i > 0; {
			select {
			case args, isOpen := <-calls:
				if !isOpen {
					i--
				}
				_ = eventSource.SendJSON("", mockscript.InvokedExecution{
					Args: args,
				})
			case se, isOpen := <-scriptError:
				if !isOpen {
					i--
				}
				var code uint8
				if se != nil {
					code, _ = interp.IsExitStatus(se)
				}
				_ = eventSource.SendJSON("", mockscript.ExecutionResult{
					ExitCode: int(code),
				})
			}
		}
	}))
	mux.Handler(http.MethodGet, "/webapp/*filepath", http.StripPrefix("/webapp", http.FileServer(http.Dir("webapp"))))
	mux.Handler(http.MethodGet, "/", http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		http.ServeFile(res, req, filepath.Join("webapp", "exec.html"))
	}))

	log.Fatal(http.ListenAndServe(":8080", mux))
}

type EventSource struct {
	id  int
	buf *bytes.Buffer
	res sse.WriteFlusher
}

func NewEventSource(res http.ResponseWriter) EventSource {
	return EventSource{
		id:  1,
		buf: bytes.NewBuffer(make([]byte, 0, 1024)),
		res: res.(sse.WriteFlusher),
	}
}

func (src *EventSource) SendJSON(event sse.EventName, data any) error {
	if event == "" {
		event = sse.EventName(reflect.TypeOf(data).Name())
	}
	buf, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = sse.Send(src.res, src.buf, src.id, event, string(buf))
	src.id++
	return err
}
