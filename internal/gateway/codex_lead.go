package gateway

import (
	"bytes"
	"strings"

	"github.com/tidwall/gjson"
)

// codexLeadTidy keeps what the ChatGPT backend says of its own account out
// of a reply to a client that isn't Codex. It says it ahead of the reply, in
// codex.rate_limits (the account's windows) and codex.response.metadata (the
// model list's tag): names only Codex has, since it reads its limits from
// there. A Responses client with a typed event enum reads an event it has no
// name for and gives up on the whole turn, so a foreign client gets the reply
// without them. Codex's own request still gets every event as it came.
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

// codexOwnEvent says an event is one of Codex's own, by its SSE name or,
// where the stream names none, by the type its data carries. A name it has
// no prefix for goes through: only codex. is the backend's own namespace.
func codexOwnEvent(ev []byte) bool {
	for _, line := range bytes.Split(ev, []byte("\n")) {
		name, ok := bytes.CutPrefix(bytes.TrimRight(line, "\r"), []byte("event:"))
		if !ok {
			continue
		}
		return strings.HasPrefix(strings.TrimSpace(string(name)), "codex.")
	}
	data := eventData(ev)
	if len(data) == 0 {
		return false
	}
	return strings.HasPrefix(gjson.GetBytes(data, "type").String(), "codex.")
}
