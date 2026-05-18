package presenters

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

type HostListRow struct {
	Alias     string `json:"alias"`
	Desc      string `json:"desc"`
	Host      string `json:"host"`
	User      string `json:"user"`
	UserRef   string `json:"user_ref"`
	Auth      string `json:"auth"`
	Port      int    `json:"port"`
	ProxyJump string `json:"proxy_jump"`
	Tags      string `json:"tags"`
	Status    string `json:"status"`
}

func RenderHostListJSON(out io.Writer, rows []HostListRow) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

func RenderHostListTable(out io.Writer, rows []HostListRow) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ALIAS\tDESC\tHOST\tUSER\tUSER_REF\tAUTH\tPORT\tPROXY_JUMP\tTAGS\tSTATUS")
	for _, row := range rows {
		fmt.Fprintf(
			w,
			"%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			row.Alias,
			row.Desc,
			row.Host,
			row.User,
			row.UserRef,
			row.Auth,
			row.Port,
			row.ProxyJump,
			row.Tags,
			row.Status,
		)
	}
	return w.Flush()
}
