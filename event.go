package pkg

type EventType int

const (
	CreateEvent EventType = iota
	UpdateEvent
	DeleteEvent
)

var eventTypeStr = [...]string{
	"CreateEvent",
	"UpdateEvent",
	"DeleteEvent",
}

func (e EventType) String() string {
	if int(e) < len(eventTypeStr) {
		return eventTypeStr[e]
	}
	return "Invalid"
}

var eventTypeVerbs = [...]string{
	"created",
	"updated",
	"deleted",
}

func (e EventType) verb() string {
	if int(e) < len(eventTypeVerbs) {
		return eventTypeVerbs[e]
	}
	return "invalid"
}

var eventTypeColor = [...]string{
	"\033[32m" + "created" + "\033[0m", // green
	"\033[33m" + "updated" + "\033[0m", // yellow
	"\033[31m" + "deleted" + "\033[0m", // red
}

func (e EventType) color() string {
	if int(e) < len(eventTypeColor) {
		return eventTypeColor[e]
	}
	return "invalid"
}

type Eventer interface {
	Event() EventType
	String() string
	Callback(c *Corpus) error
}

type Event struct {
	typ      EventType
	msg      string
	callback func(c *Corpus) error
}

func (e Event) Event() EventType { return e.typ }
func (e Event) String() string   { return e.msg }

func (e Event) Callback(c *Corpus) error {
	if e.callback == nil {
		return nil
	}
	return e.callback(c)
}
