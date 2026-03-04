// Binary entire-agent-cursor implements the external agent protocol for Cursor.
// It communicates with the Entire CLI via subcommand-based JSON over stdin/stdout.
package main

import (
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: entire-agent-cursor <subcommand> [args...]")
	}

	var err error
	switch os.Args[1] {
	// --- Core protocol ---
	case "info":
		err = cmdInfo()
	case "detect":
		err = cmdDetect()

	// --- Session management ---
	case "get-session-id":
		err = cmdGetSessionID()
	case "get-session-dir":
		err = cmdGetSessionDir()
	case "resolve-session-file":
		err = cmdResolveSessionFile()
	case "read-session":
		err = cmdReadSession()
	case "write-session":
		err = cmdWriteSession()
	case "format-resume-command":
		err = cmdFormatResumeCommand()

	// --- Transcript ---
	case "read-transcript":
		err = cmdReadTranscript()
	case "chunk-transcript":
		err = cmdChunkTranscript()
	case "reassemble-transcript":
		err = cmdReassembleTranscript()

	// --- Hooks ---
	case "parse-hook":
		err = cmdParseHook()
	case "install-hooks":
		err = cmdInstallHooks()
	case "uninstall-hooks":
		err = cmdUninstallHooks()
	case "are-hooks-installed":
		err = cmdAreHooksInstalled()

	// --- Transcript analyzer ---
	case "get-transcript-position":
		err = cmdGetTranscriptPosition()
	case "extract-modified-files":
		err = cmdExtractModifiedFiles()
	case "extract-prompts":
		err = cmdExtractPrompts()
	case "extract-summary":
		err = cmdExtractSummary()

	default:
		fatal("unknown subcommand: " + os.Args[1])
	}

	if err != nil {
		fatal(err.Error())
	}
}

// nowRFC3339 returns the current time in RFC3339 format.
func nowRFC3339() string {
	return time.Now().Format(time.RFC3339)
}
