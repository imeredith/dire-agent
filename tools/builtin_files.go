package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dire-kiwi/dire-agent/agentloop"
)

func readTool(paths pathSandbox) agentloop.Tool {
	return functionTool{
		definition: definition("read", "Read a UTF-8 text file inside the project sandbox. Relative paths resolve from the main project folder; included folders use absolute paths.", `{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer","minimum":1},"limit":{"type":"integer","minimum":1,"maximum":5000}},"required":["path"],"additionalProperties":false}`),
		execute: func(_ context.Context, raw json.RawMessage) (string, error) {
			var input struct {
				Path   string `json:"path"`
				Offset int    `json:"offset"`
				Limit  int    `json:"limit"`
			}
			if err := decode(raw, &input); err != nil {
				return "", err
			}
			path, err := paths.secureExistingPath(input.Path)
			if err != nil {
				return "", err
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			if len(contents) > maxToolOutput {
				contents = contents[:maxToolOutput]
			}
			lines := strings.Split(string(contents), "\n")
			if input.Offset <= 0 {
				input.Offset = 1
			}
			if input.Limit <= 0 {
				input.Limit = 500
			}
			start := min(input.Offset-1, len(lines))
			end := min(start+input.Limit, len(lines))
			var output strings.Builder
			for index := start; index < end; index++ {
				fmt.Fprintf(&output, "%6d\t%s\n", index+1, lines[index])
			}
			return output.String(), nil
		},
	}
}

func writeTool(paths pathSandbox) agentloop.Tool {
	return functionTool{
		definition: definition("write", "Create or overwrite a UTF-8 text file inside the project sandbox. Relative paths resolve from the main project folder; included folders use absolute paths.", `{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}`),
		execute: func(_ context.Context, raw json.RawMessage) (string, error) {
			var input struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := decode(raw, &input); err != nil {
				return "", err
			}
			path, err := paths.secureNewPath(input.Path)
			if err != nil {
				return "", err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(path, []byte(input.Content), 0o644); err != nil {
				return "", err
			}
			return fmt.Sprintf("wrote %d bytes to %s", len(input.Content), paths.displayPath(path)), nil
		},
	}
}

func editTool(paths pathSandbox) agentloop.Tool {
	return functionTool{
		definition: definition("edit", "Replace exact text in a file. By default the old text must occur exactly once.", `{"type":"object","properties":{"path":{"type":"string"},"old_text":{"type":"string"},"new_text":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["path","old_text","new_text"],"additionalProperties":false}`),
		execute: func(_ context.Context, raw json.RawMessage) (string, error) {
			var input struct {
				Path       string `json:"path"`
				OldText    string `json:"old_text"`
				NewText    string `json:"new_text"`
				ReplaceAll bool   `json:"replace_all"`
			}
			if err := decode(raw, &input); err != nil {
				return "", err
			}
			if input.OldText == "" {
				return "", errors.New("old_text must not be empty")
			}
			path, err := paths.secureExistingPath(input.Path)
			if err != nil {
				return "", err
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			count := bytes.Count(contents, []byte(input.OldText))
			if count == 0 {
				return "", errors.New("old_text was not found")
			}
			if count > 1 && !input.ReplaceAll {
				return "", fmt.Errorf("old_text occurs %d times; set replace_all or provide more context", count)
			}
			replacements := 1
			if input.ReplaceAll {
				replacements = -1
			}
			updated := bytes.Replace(contents, []byte(input.OldText), []byte(input.NewText), replacements)
			if err := os.WriteFile(path, updated, 0o644); err != nil {
				return "", err
			}
			return fmt.Sprintf("updated %s (%d replacement(s))", paths.displayPath(path), count), nil
		},
	}
}
