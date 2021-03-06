package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/newalchemylimited/seth"
)

func client() *seth.Client {
	url := os.Getenv("SETH_URL")
	if url == "" {
		debugf("SETH_URL unset; trying to dial local client")
		return seth.NewClient(seth.IPCDial)
	}
	if strings.HasPrefix(url, "http") {
		var t seth.Transport
		if strings.Contains(url, "api.infura.io") {
			debugf("interpreted %q as api.infura.io", url)
			t = seth.InfuraTransport{}
		} else {
			debugf("using http transport %q", url)
			t = &seth.HTTPTransport{URL: url}
		}
		return seth.NewClientTransport(t)
	}
	if _, err := os.Stat(url); err == nil {
		debugf("using IPC path %s", url)
		return seth.NewClient(seth.IPCPath(url))
	}
	fmt.Fprintf(os.Stderr, "cannot derive client from SETH_URL=%q\n", url)
	os.Exit(1)
	return nil
}

// blocknum converts a text string into a block
// number, respecting the "earliest," "latest,"
// and "pending" conventions
func blocknum(s string) int64 {
	switch s {
	case "earliest":
		return 0
	case "latest":
		return seth.Latest
	case "pending":
		return seth.Pending
	default:
		bn, err := strconv.ParseInt(s, 0, 64)
		if err != nil {
			fatalf("bad block specifier %q: %s\n", s, err)
		}
		return bn
	}
}

func getblock(c *seth.Client, num int64, txs bool) *seth.Block {
	b, err := c.GetBlock(num, txs)
	if err != nil {
		fatalf("getting block: %s\n", err)
	}
	return b
}

func getcode(c *seth.Client, addr *seth.Address) []byte {
	code, err := c.GetCode(addr)
	if err != nil {
		fatalf("getting code: %s\n", err)
	}
	return code
}
