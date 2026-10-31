package gateway

import (
	"bytes"
	"strings"

	"github.com/tidwall/gjson"
)

// codexLeadTidy keeps what the ChatGPT backend says in the Codex protocol
// out of a reply to a client that isn't Codex. It says it ahead of the reply
// or beside it, in codex.rate_limits (the account's windows),
// codex.response.metadata (the model list's tag) and response.metadata (the
// turn's headers on a socket, its moderation word on the HTTP route): names
// only Codex reads. A Responses client with a typed event enum reads one it
// has no name for and gives up on the whole turn, so a foreign client gets
// the reply without them. Codex's own request still gets every event as it
// came.
type codexLeadTidy struct {
	buf []byte
}

func (t *codexLeadTidy) write(b []byte) []byte {
	t.buf = append(t.buf, b...)
	out, rest := tidyEvents(t.buf, t.event)
	t.buf = rest
	return out
}

func (t *codexLeadTidy) flush() []byte {
	b := t.buf
	t.buf = nil
	return b
}

// event is one whole SSE event, dropped when it is Codex's own.
func (t *codexLeadTidy) event(ev []byte) []byte {
	if codexOwnEvent(ev) {
		return nil
	}
	return ev
}

// codexOwnEvent says an event is one Codex reads from the backend itself,
// by its SSE name or, where the stream names none, by the type its data
// carries. What Codex has of its own is response.metadata, the turn's word
// on itself, and the codex. namespace; a name in neither goes through.
func codexOwnEvent(ev []byte) bool {
	name := ""
	for _, line := range bytes.Split(ev, []byte("\n")) {
		v, ok := bytes.CutPrefix(bytes.TrimRight(line, "\r"), []byte("event:"))
		if !ok {
			continue
		}
		name = strings.TrimSpace(string(v))
		break
	}
	if name == "" {
		data := eventData(ev)
		if len(data) == 0 {
			return false
		}
		name = gjson.GetBytes(data, "type").String()
	}
	return strings.HasPrefix(name, "codex.") || name == "response.metadata"
}
