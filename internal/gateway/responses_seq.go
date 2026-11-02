package gateway

import "github.com/tidwall/gjson"

// sequenceWatch follows the sequence_number a Responses stream has sent, so an
// event magpie adds after the upstream's last one continues the numbering. A
// typed client reads sequence_number as required on every event, and its
// events are numbered in one run: an event magpie writes itself (the
// response.failed that ends a cut stream) has to carry the next one, not a
// number of its own.
type sequenceWatch struct {
	buf  []byte
	last int // the last sequence_number that went out, -1 before the first
}

func (s *sequenceWatch) write(b []byte) []byte {
	s.buf = append(s.buf, b...)
	out, rest := tidyEvents(s.buf, s.event)
	s.buf = rest
	return out
}

func (s *sequenceWatch) flush() []byte {
	b := s.buf
	s.buf = nil
	return b
}

// event is one whole SSE event, let through as it came once its number is noted.
func (s *sequenceWatch) event(ev []byte) []byte {
	if n := gjson.GetBytes(eventData(ev), "sequence_number"); n.Type == gjson.Number {
		if v := int(n.Int()); v > s.last {
			s.last = v
		}
	}
	return ev
}

// next is the number an event added after what has gone out takes.
func (s *sequenceWatch) next() int { return s.last + 1 }
