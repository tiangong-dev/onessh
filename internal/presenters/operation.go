package presenters

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

type BatchResult struct {
	Alias  string
	Skip   bool
	Err    error
	Stdout []byte
	Stderr []byte
}

func RenderBatchResults(out, errOut io.Writer, aliases []string, results []BatchResult) bool {
	anyFailed := false
	for i, alias := range aliases {
		r := results[i]
		if r.Skip {
			fmt.Fprintf(errOut, "SKIP %s: %v\n", alias, r.Err)
			continue
		}
		if len(r.Stdout) > 0 || len(r.Stderr) > 0 {
			fmt.Fprintf(out, "=== %s ===\n", alias)
			if len(r.Stdout) > 0 {
				if _, err := out.Write(r.Stdout); err != nil {
					fmt.Fprintf(errOut, "write stdout for %s: %v\n", alias, err)
					anyFailed = true
				}
			}
			if len(r.Stderr) > 0 {
				if _, err := errOut.Write(r.Stderr); err != nil {
					fmt.Fprintf(errOut, "write stderr for %s: %v\n", alias, err)
					anyFailed = true
				}
			}
		}
		if r.Err != nil {
			if len(r.Stdout) == 0 && len(r.Stderr) == 0 {
				fmt.Fprintf(out, "%-20s  FAIL\n", alias)
			} else {
				fmt.Fprintf(errOut, "FAIL %s: %v\n", alias, r.Err)
			}
			anyFailed = true
		} else if len(r.Stdout) == 0 && len(r.Stderr) == 0 {
			fmt.Fprintf(out, "%-20s  OK\n", alias)
		}
	}
	return anyFailed
}

type DryRunHost struct {
	Alias     string
	Host      string
	User      string
	Port      int
	SkipError string
}

func RenderDryRunHosts(out io.Writer, hosts []DryRunHost) error {
	if _, err := fmt.Fprintf(out, "Matched %d host(s):\n", len(hosts)); err != nil {
		return err
	}
	for _, host := range hosts {
		if host.SkipError != "" {
			if _, err := fmt.Fprintf(out, "  %-20s %s (SKIP: %s)\n", host.Alias, host.Host, host.SkipError); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(out, "  %-20s %s@%s:%d\n", host.Alias, host.User, host.Host, host.Port); err != nil {
			return err
		}
	}
	return nil
}

func RenderDryRunCommand(out io.Writer, command []string) error {
	_, err := fmt.Fprintf(out, "Command: %s\n", strings.Join(command, " "))
	return err
}

func RenderDryRunUpload(out io.Writer, localPaths []string, remotePath string) error {
	_, err := fmt.Fprintf(out, "Upload: %s -> :%s\n", strings.Join(localPaths, ", "), remotePath)
	return err
}

type AuditLogEvent struct {
	Time   string            `json:"time"`
	Action string            `json:"action"`
	Alias  string            `json:"alias,omitempty"`
	Host   string            `json:"host,omitempty"`
	User   string            `json:"user,omitempty"`
	Result string            `json:"result"`
	Error  string            `json:"error,omitempty"`
	Extra  map[string]string `json:"extra,omitempty"`
}

func RenderAuditLog(out io.Writer, events []AuditLogEvent, format string) error {
	if len(events) == 0 {
		_, err := fmt.Fprintln(out, "No audit log entries.")
		return err
	}
	if format == "json" {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(events)
	}
	if format != "table" {
		return fmt.Errorf("unsupported audit log format %q", format)
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TIME\tACTION\tALIAS\tHOST\tUSER\tRESULT\tERROR")
	for _, e := range events {
		errMsg := "-"
		if e.Error != "" {
			errMsg = e.Error
		}
		aliasCol := e.Alias
		if aliasCol == "" {
			aliasCol = "-"
		}
		hostCol := e.Host
		if hostCol == "" {
			hostCol = "-"
		}
		userCol := e.User
		if userCol == "" {
			userCol = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			e.Time, e.Action, aliasCol, hostCol, userCol, e.Result, errMsg)
	}
	return w.Flush()
}
