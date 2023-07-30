package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/alevinval/sse/pkg/decoder"
	"github.com/bgentry/go-netrc/netrc"
)

type Response struct {
	Completion string `json:"completion"`
	StopReason string `json:"stop_reason"`
	Model      string `json:"model"`
}
type Error struct {
	Details ErrorDetails `json:"error"`
}
type ErrorDetails struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func formatPrompt(prompt string) string {
	return fmt.Sprintf("\n\nHuman: %s\n\nAssistant:", prompt)
}

func doClaude() error {
	var model string
	var max_tokens int
	var temperature float64
	var top_p float64
	var raw bool

	flag.StringVar(&model, "model", "claude-2", "model name")
	flag.IntVar(&max_tokens, "max-tokens", 256, "max tokens to sample")
	flag.Float64Var(&temperature, "temperature", -1.0, "sample temperature")
	flag.Float64Var(&top_p, "top-p", -1.0, "sample top-p")
	flag.BoolVar(&raw, "raw", false, "Do not format prompt in Human/Assistant format")

	flag.Parse()

	rc, err := netrc.ParseFile(os.ExpandEnv("$HOME/.netrc"))
	if err != nil {
		return fmt.Errorf("netrc: %w", err)
	}
	m := rc.FindMachine("api.anthropic.com")
	if m == nil {
		return fmt.Errorf("no credentials for api.anthropic.com")
	}
	key := m.Password
	prompt := flag.Arg(0)
	if prompt == "" {
		return fmt.Errorf("No prompt given")
	}

	if !raw {
		prompt = formatPrompt(prompt)
	}

	reqParams := map[string]interface{}{
		"model":                model,
		"prompt":               prompt,
		"max_tokens_to_sample": max_tokens,
		"stream":               true,
	}

	if temperature >= 0 {
		reqParams["temperature"] = temperature
	}
	if top_p >= 0 {
		reqParams["top_p"] = top_p
	}

	body, err := json.Marshal(reqParams)
	if err != nil {
		return fmt.Errorf("can't marshal args: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/complete", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("NewRequest: %w", err)
	}

	req.Header.Set("accept", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", key)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST: %w", err)
	}
	defer resp.Body.Close()

	/*
		var reply Response
		if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
			return fmt.Errorf("decode: %w", err)
		}

		fmt.Printf("%s\n", reply.Completion)
	*/
	events := decoder.New(resp.Body)
	for {
		event, err := events.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("decoding: %w", err)
		}

		switch event.Name {
		case "completion":
			var msg Response
			if err := json.Unmarshal([]byte(event.Data), &msg); err != nil {
				return fmt.Errorf("parse %q: %w", event.Data, err)
			}
			fmt.Print(msg.Completion)
		case "error":
			var msg Error
			if err := json.Unmarshal([]byte(event.Data), &msg); err != nil {
				return fmt.Errorf("parse %q: %w", event.Data, err)
			}
			fmt.Fprintf(os.Stderr, "Error code=%s: %q", msg.Details.Type, msg.Details.Message)
		}
	}
	fmt.Println("")

	return nil
}

func main() {
	err := doClaude()
	if err != nil {
		log.Fatal(err.Error())
	}
}
