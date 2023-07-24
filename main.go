package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"time"

	"github.com/alecthomas/kong"
	"github.com/sourcegraph/go-lsp"
	"github.com/sourcegraph/jsonrpc2"
)

type CLI struct {
	File string `arg:"" type:"path" help:"File to analyze."`
}

func main() {
	var cli CLI
	kong.Parse(&cli)

	// Start the Go language server
	conn, err := startGoLanguageServer()
	if err != nil {
		log.Fatal(err)
	}

	path := "/Users/bakks/drillsp"
	// Initialize the language server
	if err := initializeLanguageServer(path, conn); err != nil {
		log.Fatal(err)
	}
	if err := initializedLanguageServer(conn); err != nil {
		log.Fatal(err)
	}

	uri := lsp.DocumentURI("file://" + cli.File)
	text, err := ioutil.ReadFile(cli.File)
	if err != nil {
		log.Fatal(err)
	}

	didOpenFile(conn, uri, string(text))

	// Send a textDocument/documentSymbol request
	req := lsp.DocumentSymbolParams{
		TextDocument: lsp.TextDocumentIdentifier{
			URI: uri,
		},
	}

	log.Printf("Fetching document symbols for %s ...", uri)
	var symbols []lsp.SymbolInformation
	if err := conn.Call(context.Background(), "textDocument/documentSymbol", req, &symbols); err != nil {
		log.Fatal(err)
	}
	log.Printf("Fetched")

	// Print the function names
	for _, symbol := range symbols {
		if symbol.Kind == lsp.SKFunction {
			fmt.Println(symbol.Name)
		}
	}
}

// func to initialize the LSP server over jsonrpc2
func initializeLanguageServer(path string, conn *jsonrpc2.Conn) error {
	log.Printf("Initializing LSP server...")
	req := lsp.InitializeParams{
		RootURI: lsp.DocumentURI("file://" + path),
	}

	var resp lsp.InitializeResult
	if err := conn.Call(context.Background(), "initialize", req, &resp); err != nil {
		return err
	}

	log.Printf("Got initialize response with capabilities")

	return nil
}

// send initialized notification
func initializedLanguageServer(conn *jsonrpc2.Conn) error {
	log.Printf("Sending initialized notification...")

	// pass an empty param map because of this bug:
	// https://github.com/golang/go/issues/57459
	if err := conn.Notify(context.Background(), "initialized", map[string]any{}); err != nil {
		return err
	}
	log.Printf("Sent")

	return nil
}

// func to send a didopen on the target file
func didOpenFile(conn *jsonrpc2.Conn, uri lsp.DocumentURI, text string) error {
	log.Printf("Sending didOpen for %s ...", uri)
	// Send a textDocument/didOpen notification
	notification := lsp.DidOpenTextDocumentParams{
		TextDocument: lsp.TextDocumentItem{
			URI:        uri,
			LanguageID: "go",
			Version:    1,
			Text:       text,
		},
	}

	if err := conn.Notify(context.Background(), "textDocument/didOpen", notification); err != nil {
		return err
	}
	log.Printf("Sent")

	return nil
}

type LSPConnection struct {
}

func (this *LSPConnection) Handle(ctx context.Context, conn *jsonrpc2.Conn, request *jsonrpc2.Request) {
	params := request.Params

	// parse json as a lsp.ShowMessageParams message
	var showMessageParams lsp.ShowMessageParams
	if err := json.Unmarshal(*params, &showMessageParams); err != nil {
		log.Printf("Error parsing message: %s", err)
		return
	}

	var prefix string

	switch showMessageParams.Type {
	case lsp.MTError:
		prefix = "Error"
	case lsp.MTWarning:
		prefix = "Warning"
	case lsp.Info:
		prefix = "Info"
	case lsp.Log:
		prefix = "Log"
	default:
		panic("unexpected message type")
	}

	log.Printf("Server notification %s %s %s: %s", request.Method, request.ID, prefix, showMessageParams.Message)
}

func startGoLanguageServer() (*jsonrpc2.Conn, error) {
	log.Printf("Starting Golang LSP server...")
	cmd := exec.Command("gopls", "-logfile=./gopls.log", "-rpc.trace", "-vv", "-mode=stdio")
	cmd.Env = os.Environ()

	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	go io.Copy(os.Stderr, stderr)

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	outReader := &readerLogger{out}
	inWriter := &writerLogger{in}
	netConn := readWriteNetConn{inWriter, outReader}

	stream := jsonrpc2.NewBufferedStream(netConn, jsonrpc2.VSCodeObjectCodec{})
	lspConn := &LSPConnection{}
	conn := jsonrpc2.NewConn(context.Background(), stream, lspConn)
	log.Printf("Started")

	// goroutine to check if the command exits
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Printf("Command exited with error: %s", err)
		} else {
			log.Printf("Command exited")
		}
	}()

	return conn, nil
}

// implements net.Conn interface
type readWriteNetConn struct {
	io.Writer
	io.Reader
}

func (readWriteNetConn) Close() error                       { panic("unimplemented") }
func (readWriteNetConn) LocalAddr() net.Addr                { panic("unimplemented") }
func (readWriteNetConn) RemoteAddr() net.Addr               { panic("unimplemented") }
func (readWriteNetConn) SetDeadline(t time.Time) error      { panic("unimplemented") }
func (readWriteNetConn) SetReadDeadline(t time.Time) error  { panic("unimplemented") }
func (readWriteNetConn) SetWriteDeadline(t time.Time) error { panic("unimplemented") }

// this implements io.Reader and logs calls to Read before forwarding on the read
type readerLogger struct {
	io.Reader
}

func (this *readerLogger) Read(p []byte) (int, error) {
	n, err := this.Reader.Read(p)
	//log.Printf("Read %d bytes: %s", n, string(p))
	return n, err
}

// this implements io.Writer and logs calls to Write before forwarding on the write
type writerLogger struct {
	io.Writer
}

func (this *writerLogger) Write(p []byte) (int, error) {
	//log.Printf("Write %d bytes: %s", len(p), string(p))
	n, err := this.Writer.Write(p)

	return n, err
}
