package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/internal/tokencount"
)

const tokencountMaxURLBody = 32 << 20

func main() {
	runTokenCount(os.Args[1:])
}

func runTokenCount(args []string) {
	fs := flag.NewFlagSet("tokencount", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var file, urlStr, str string
	fs.StringVar(&file, "file", "", "read text from `path`")
	fs.StringVar(&file, "f", "", "read text from `path` (shorthand)")
	fs.StringVar(&urlStr, "url", "", "fetch text from `URL` (HTTP GET)")
	fs.StringVar(&str, "string", "", "count tokens in `text`")
	fs.StringVar(&str, "s", "", "count tokens in `text` (shorthand)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage:
  tokencount [flags] [arg ...]
  tokencount -                     read from stdin
  echo "hello" | tokencount

Prints byte size of the input, then token counts for cl100k_base and o200k_base (tiktoken-compatible).

Input (first match wins among explicit flags, then positionals, then stdin):

  -file / -f path     read file contents
  -url URL            HTTP GET body (max %d MiB)
  -string / -s text   use literal string

  With no flags:
    - One argument "-"   -> stdin
    - One other argument -> if it names an existing regular file, read it; else treat as literal text
    - Several arguments  -> join with spaces and count as literal text
    - No arguments       -> read stdin when piped or redirected; otherwise show this help

`, tokencountMaxURLBody>>20)
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		os.Exit(2)
	}
	pos := fs.Args()

	nFlags := 0
	if file != "" {
		nFlags++
	}
	if urlStr != "" {
		nFlags++
	}
	if str != "" {
		nFlags++
	}
	if nFlags > 1 {
		fmt.Fprintln(os.Stderr, "tokencount: use at most one of -file, -url, -string")
		os.Exit(2)
	}
	if nFlags == 1 && len(pos) > 0 {
		fmt.Fprintln(os.Stderr, "tokencount: extra arguments cannot be combined with -file, -url, or -string")
		os.Exit(2)
	}

	var input []byte
	var err error
	switch {
	case file != "":
		input, err = os.ReadFile(file)
	case urlStr != "":
		input, err = fetchURL(urlStr)
	case str != "":
		input = []byte(str)
	case len(pos) > 0:
		if len(pos) == 1 && pos[0] == "-" {
			input, err = io.ReadAll(os.Stdin)
		} else if len(pos) == 1 {
			info, statErr := os.Stat(pos[0])
			if statErr == nil && !info.IsDir() && info.Mode().IsRegular() {
				input, err = os.ReadFile(pos[0])
			} else {
				input = []byte(pos[0])
			}
		} else {
			input = []byte(strings.Join(pos, " "))
		}
	default:
		if stdinLooksRedirected() {
			input, err = io.ReadAll(os.Stdin)
		} else {
			fs.Usage()
			os.Exit(2)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "tokencount: %v\n", err)
		os.Exit(1)
	}

	s := string(input)
	cl, err := tokencount.Count(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tokencount: cl100k_base: %v\n", err)
		os.Exit(1)
	}
	o2, err := tokencount.CountEncoding(tokencount.EncodingO200kBase, s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "tokencount: o200k_base: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("bytes\t%d\n", len(input))
	fmt.Printf("cl100k_base\t%d\n", cl)
	fmt.Printf("o200k_base\t%d\n", o2)
}

func stdinLooksRedirected() bool {
	st, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice == 0
}

func fetchURL(raw string) ([]byte, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return nil, fmt.Errorf("empty URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %s", res.Status)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, tokencountMaxURLBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > tokencountMaxURLBody {
		return nil, fmt.Errorf("response larger than %d bytes", tokencountMaxURLBody)
	}
	return body, nil
}
