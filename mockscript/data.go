package mockscript

type InvokedExecution struct {
	Args []string `json:"args,omitempty"`
}

type ExecutionResult struct {
	ExitCode int `json:"exitCode"`
}
