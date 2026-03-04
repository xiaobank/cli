// Binary entire-agent-cursor implements the external agent protocol for the Cursor IDE.
// It is discovered via PATH by the CLI's external.DiscoverAndRegister().
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: entire-agent-cursor <subcommand> [args...]")
	}

	var err error
	switch os.Args[1] {
	case "info":
		err = cmdInfo()
	case "detect":
		err = cmdDetect()
	case "get-session-id":
		err = cmdGetSessionID()
	case "get-session-dir":
		err = runGetSessionDir()
	case "resolve-session-file":
		err = runResolveSessionFile()
	case "read-session":
		err = cmdReadSession()
	case "write-session":
		err = cmdWriteSession()
	case "format-resume-command":
		err = cmdFormatResumeCommand()
	case "read-transcript":
		err = runReadTranscript()
	case "chunk-transcript":
		err = runChunkTranscript()
	case "reassemble-transcript":
		err = cmdReassembleTranscript()
	case "parse-hook":
		err = runParseHook()
	case "install-hooks":
		err = runInstallHooks()
	case "uninstall-hooks":
		err = cmdUninstallHooks()
	case "are-hooks-installed":
		err = cmdAreHooksInstalled()
	case "get-transcript-position":
		err = runGetTranscriptPosition()
	case "extract-modified-files":
		err = cmdExtractModifiedFiles()
	case "extract-prompts":
		err = runExtractPrompts()
	case "extract-summary":
		err = runExtractSummary()
	default:
		fatal("unknown subcommand: " + os.Args[1])
	}

	if err != nil {
		fatal(err.Error())
	}
}

func cmdInfo() error {
	return writeJSON(map[string]interface{}{
		"protocol_version": 1,
		"name":             agentName,
		"type":             "Cursor",
		"description":      "Cursor - AI-powered code editor",
		"is_preview":       true,
		"protected_dirs":   []string{".cursor"},
		"hook_names": []string{
			hookNameSessionStart,
			hookNameSessionEnd,
			hookNameBeforeSubmitPrompt,
			hookNameStop,
			hookNamePreCompact,
			hookNameSubagentStart,
			hookNameSubagentStop,
		},
		"capabilities": map[string]bool{
			"hooks":                    true,
			"transcript_analyzer":      true,
			"transcript_preparer":      false,
			"token_calculator":         false,
			"text_generator":           false,
			"hook_response_writer":     false,
			"subagent_aware_extractor": false,
		},
	})
}

func runGetSessionDir() error {
	fs := flag.NewFlagSet("get-session-dir", flag.ContinueOnError)
	repoPath := fs.String("repo-path", "", "repository path")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	return cmdGetSessionDir(*repoPath)
}

func runResolveSessionFile() error {
	fs := flag.NewFlagSet("resolve-session-file", flag.ContinueOnError)
	sessionDir := fs.String("session-dir", "", "session directory")
	sessionID := fs.String("session-id", "", "session ID")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	return cmdResolveSessionFile(*sessionDir, *sessionID)
}

func runReadTranscript() error {
	fs := flag.NewFlagSet("read-transcript", flag.ContinueOnError)
	sessionRef := fs.String("session-ref", "", "session reference path")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	return cmdReadTranscript(*sessionRef)
}

func runChunkTranscript() error {
	fs := flag.NewFlagSet("chunk-transcript", flag.ContinueOnError)
	maxSize := fs.Int("max-size", 0, "maximum chunk size in bytes")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	return cmdChunkTranscript(*maxSize)
}

func runParseHook() error {
	fs := flag.NewFlagSet("parse-hook", flag.ContinueOnError)
	hookName := fs.String("hook", "", "hook name")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	return cmdParseHook(*hookName)
}

func runInstallHooks() error {
	fs := flag.NewFlagSet("install-hooks", flag.ContinueOnError)
	localDev := fs.Bool("local-dev", false, "use local dev commands")
	force := fs.Bool("force", false, "remove existing Entire hooks first")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	return cmdInstallHooks(*localDev, *force)
}

func runGetTranscriptPosition() error {
	fs := flag.NewFlagSet("get-transcript-position", flag.ContinueOnError)
	path := fs.String("path", "", "transcript file path")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	return cmdGetTranscriptPosition(*path)
}

func runExtractPrompts() error {
	fs := flag.NewFlagSet("extract-prompts", flag.ContinueOnError)
	sessionRef := fs.String("session-ref", "", "session reference path")
	offset := fs.String("offset", "0", "line offset")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	fromOffset, err := strconv.Atoi(*offset)
	if err != nil {
		return fmt.Errorf("invalid offset: %w", err)
	}
	return cmdExtractPrompts(*sessionRef, fromOffset)
}

func runExtractSummary() error {
	fs := flag.NewFlagSet("extract-summary", flag.ContinueOnError)
	sessionRef := fs.String("session-ref", "", "session reference path")
	if err := fs.Parse(os.Args[2:]); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}
	return cmdExtractSummary(*sessionRef)
}

// writeJSON marshals v as JSON and writes it to stdout.
func writeJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	if _, err := os.Stdout.Write(data); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}
	return nil
}

// writeNull writes the JSON null literal to stdout.
func writeNull() error {
	if _, err := os.Stdout.Write([]byte("null")); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}
	return nil
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
