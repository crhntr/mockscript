run: webapp/webapp.wasm webapp/wasm_exec.js
	go run ./ testdata/show_exec.sh

webapp/webapp.wasm: ./webapp
	mkdir -p ./server/assets
	GOOS=js GOARCH=wasm go build -o webapp/webapp.wasm ./webapp/

webapp/wasm_exec.js:
	cp "$$(go env GOROOT)/misc/wasm/wasm_exec.js" webapp/wasm_exec.js
