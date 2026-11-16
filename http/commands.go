package fbhttp

import (
	"bufio"
	"context"
	"io"
	"log"
	"net/http"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"github.com/filebrowser/filebrowser/v2/runner"
)

const (
	WSWriteDeadline    = 10 * time.Second
	maxCommandBytes    = 16 << 10
	commandReadTimeout = 30 * time.Second
	commandRunTimeout  = 10 * time.Minute
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var (
	cmdNotAllowed = []byte("Command not allowed.")
)

func wsErr(ws *websocket.Conn, r *http.Request, status int, err error) {
	txt := http.StatusText(status)
	if err != nil || status >= 400 {
		log.Printf("%s: %v %s %v", r.URL.Path, status, r.RemoteAddr, err)
	}
	if err := ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, txt), time.Now().Add(WSWriteDeadline)); err != nil {
		log.Print(err)
	}
}

func writeCommandMessage(conn *websocket.Conn, body []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(WSWriteDeadline)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, body)
}

var commandsHandler = withUser(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if !d.server.EnableExec || !d.user.Perm.Execute {
		return http.StatusForbidden, nil
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	defer conn.Close()
	conn.SetReadLimit(maxCommandBytes)
	if err := conn.SetReadDeadline(time.Now().Add(commandReadTimeout)); err != nil {
		return 0, err
	}

	var raw string

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			wsErr(conn, r, http.StatusInternalServerError, err)
			return 0, nil
		}

		raw = strings.TrimSpace(string(msg))
		if raw != "" {
			break
		}
	}

	command, name, err := runner.ParseCommand(d.settings, raw)
	if err != nil {
		if err := writeCommandMessage(conn, []byte(err.Error())); err != nil {
			wsErr(conn, r, http.StatusInternalServerError, err)
		}
		return 0, nil
	}

	if !slices.Contains(d.user.Commands, name) {
		if err := writeCommandMessage(conn, cmdNotAllowed); err != nil {
			wsErr(conn, r, http.StatusInternalServerError, err)
		}

		return 0, nil
	}

	ctx, cancel := context.WithTimeout(r.Context(), commandRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = d.user.FullPath(r.URL.Path)
	cmd.WaitDelay = 2 * time.Second
	output, writer := io.Pipe()
	defer output.Close()
	defer writer.Close()
	cmd.Stdout, cmd.Stderr = writer, writer

	if err := cmd.Start(); err != nil {
		wsErr(conn, r, http.StatusInternalServerError, err)
		return 0, nil
	}
	go func() { _ = writer.CloseWithError(cmd.Wait()) }()

	s := bufio.NewScanner(output)
	s.Buffer(make([]byte, 4096), 256<<10)
	for s.Scan() {
		if err := writeCommandMessage(conn, s.Bytes()); err != nil {
			log.Print(err)
			return 0, nil
		}
	}

	if err := s.Err(); err != nil {
		wsErr(conn, r, http.StatusInternalServerError, err)
	}

	return 0, nil
})
