package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dire-kiwi/dire-agent/agentloop"
)

func listTool(paths pathSandbox) agentloop.Tool {
	return functionTool{
		definition: definition("ls", "List a directory inside the project sandbox. Relative paths resolve from the main project folder; included folders use absolute paths.", `{"type":"object","properties":{"path":{"type":"string"}},"additionalProperties":false}`),
		execute: func(_ context.Context, raw json.RawMessage) (string, error) {
			var input struct {
				Path string `json:"path"`
			}
			if err := decode(raw, &input); err != nil {
				return "", err
			}
			if input.Path == "" {
				input.Path = "."
			}
			path, err := paths.secureExistingPath(input.Path)
			if err != nil {
				return "", err
			}
			entries, err := os.ReadDir(path)
			if err != nil {
				return "", err
			}
			var output strings.Builder
			for _, entry := range entries {
				name := entry.Name()
				if entry.IsDir() {
					name += "/"
				}
				output.WriteString(name + "\n")
			}
			return output.String(), nil
		},
	}
}

func findTool(paths pathSandbox) agentloop.Tool {
	return functionTool{
		definition: definition("find", "Find files by a filepath glob inside the project sandbox. Relative paths resolve from the main project folder; included folders use absolute paths.", `{"type":"object","properties":{"path":{"type":"string"},"pattern":{"type":"string"},"max_results":{"type":"integer","minimum":1,"maximum":1000}},"additionalProperties":false}`),
		execute: func(_ context.Context, raw json.RawMessage) (string, error) {
			var input struct {
				Path       string `json:"path"`
				Pattern    string `json:"pattern"`
				MaxResults int    `json:"max_results"`
			}
			if err := decode(raw, &input); err != nil {
				return "", err
			}
			if input.Path == "" {
				input.Path = "."
			}
			if input.Pattern == "" {
				input.Pattern = "*"
			}
			if input.MaxResults <= 0 {
				input.MaxResults = 200
			}
			start, err := paths.secureExistingPath(input.Path)
			if err != nil {
				return "", err
			}
			var matches []string
			err = filepath.WalkDir(start, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return nil
				}
				if len(matches) >= input.MaxResults {
					return fs.SkipAll
				}
				rel := paths.displayPath(path)
				matched, _ := filepath.Match(input.Pattern, entry.Name())
				if !matched {
					matched, _ = filepath.Match(input.Pattern, rel)
				}
				if matched {
					if entry.IsDir() {
						rel += "/"
					}
					matches = append(matches, rel)
				}
				return nil
			})
			return strings.Join(matches, "\n"), err
		},
	}
}

func grepTool(paths pathSandbox) agentloop.Tool {
	return functionTool{
		definition: definition("grep", "Search UTF-8 files with a regular expression inside the project sandbox. Relative paths resolve from the main project folder; included folders use absolute paths.", `{"type":"object","properties":{"path":{"type":"string"},"pattern":{"type":"string"},"max_results":{"type":"integer","minimum":1,"maximum":1000}},"required":["pattern"],"additionalProperties":false}`),
		execute: func(_ context.Context, raw json.RawMessage) (string, error) {
			var input struct {
				Path       string `json:"path"`
				Pattern    string `json:"pattern"`
				MaxResults int    `json:"max_results"`
			}
			if err := decode(raw, &input); err != nil {
				return "", err
			}
			expression, err := regexp.Compile(input.Pattern)
			if err != nil {
				return "", err
			}
			if input.Path == "" {
				input.Path = "."
			}
			if input.MaxResults <= 0 {
				input.MaxResults = 200
			}
			start, err := paths.secureExistingPath(input.Path)
			if err != nil {
				return "", err
			}
			var matches []string
			err = filepath.WalkDir(start, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil || entry.IsDir() {
					return nil
				}
				if len(matches) >= input.MaxResults {
					return fs.SkipAll
				}
				file, err := os.Open(path)
				if err != nil {
					return nil
				}
				scanner := bufio.NewScanner(io.LimitReader(file, maxToolOutput))
				line := 0
				for scanner.Scan() {
					line++
					if expression.MatchString(scanner.Text()) {
						matches = append(matches, fmt.Sprintf("%s:%d:%s", paths.displayPath(path), line, scanner.Text()))
						if len(matches) >= input.MaxResults {
							break
						}
					}
				}
				_ = file.Close()
				return nil
			})
			return strings.Join(matches, "\n"), err
		},
	}
}
