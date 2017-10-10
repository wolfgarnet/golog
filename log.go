package golog

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	ExpandFunctionName = "FUNCTION"
	ExpandLineNumber   = "LINE"
	ExpandTime         = "TIME"
	ExpandDuration     = "DURATION"
	ExpandSubject      = "SUBJECT"
	ExpandMessageLevel = "LEVEL"
	ExpandMessage      = "MESSAGE"
)

type LogLevel uint8

const (
	All LogLevel = iota
	Trace
	Debug
	Info
	Warning
	Error
	None
)

func (ll LogLevel) String() string {
	switch ll {
	case None:
		return "NONE"
	case Error:
		return "ERR "
	case Warning:
		return "WARN"
	case Info:
		return "INFO"
	case Debug:
		return "DBG "
	case Trace:
		return "TRC "
	case All:
		return "ALL "

	default:
		panic(fmt.Sprintf("No such log level, %v", ll))
	}
}

type Instance struct {
	Name         string
	Format       string
	output       io.Writer
	tags         []string
	log          *log.Logger
	needsRuntime bool

	UpdateOutput        func()
	UpdateOutputTrigger time.Ticker
}

func newInstance(name, format string, output io.Writer, tags ...string) *Instance {
	i := &Instance{
		Name:   name,
		Format: format,
		output: output,
		tags:   tags,
	}

	return i.checkForRuntime()
}

func (i *Instance) checkForRuntime() *Instance {
	if strings.Contains(i.Format, ExpandFunctionName) || strings.Contains(i.Format, ExpandLineNumber) {
		i.needsRuntime = true
	}

	return i
}

func (i *Instance) expand(format string, msg *Message) string {
	for expand, value := range msg.values {
		format = strings.Replace(format, expand, value, -1)
	}

	return format
}

func (i *Instance) initialize() *Instance {
	//i.log = log.New(os.Stdout, "", log.LstdFlags)
	i.log = log.New(i.output, "", 0)
	return i
}

func AddLoggerInstance(name, format string, output io.Writer, tags ...string) {
	RegisterLogger <- newInstance(name, format, output, tags...).initialize()
}

type Message struct {
	Level    LogLevel
	Subject  interface{}
	Tags     []string
	values   map[string]string
	Format   string
	Elements []interface{}
}

var (
	// Sink is the main channel for sending/recieving messages
	Sink chan Message
	// NewLevel is channel that can change the log level
	NewLevel chan LogLevel
	level    LogLevel = Info

	// By default there is one logging instance, and it logs to stdout.
	gologgers []*Instance = []*Instance{newInstance("default", "[LEVEL] SUBJECT, MESSAGE", os.Stdout).initialize()}

	RegisterLogger   chan *Instance
	DeRegisterLogger chan string

	// Register the start time
	start = time.Now()
)

func Log(level LogLevel, subject interface{}, format string, elements ...interface{}) {
	pc, name, line, _ := runtime.Caller(1)
	fun := runtime.FuncForPC(pc)
	if fun != nil {
		name = fun.Name()
	}

	// Context specific values
	values := map[string]string{
		ExpandLineNumber:   strconv.Itoa(line),
		ExpandFunctionName: name,
		ExpandTime:         time.Now().String(),
		ExpandDuration:     time.Now().Sub(start).String(),
	}

	Sink <- Message{level, subject, nil, values, format, elements}
}

func IsProduction(b bool) {
	if b {
		NewLevel <- Warning
	}
}

func init() {
	Sink = make(chan Message, 256)
	RegisterLogger = make(chan *Instance, 1)
	DeRegisterLogger = make(chan string, 1)
	NewLevel = make(chan LogLevel)

	go func() {
		for {
			select {
			case logger := <-RegisterLogger:
				gologgers = append(gologgers, logger)
			case name := <-DeRegisterLogger:
				i := 0
				for _, l := range gologgers {
					if l.Name == name {
						continue
					}
					gologgers[i] = l
					i++
				}
				gologgers = gologgers[:i]
			case newLevel := <-NewLevel:
				level = newLevel
			case msg := <-Sink:
				if msg.Level < level {
					break
				}

				var loggers []*Instance

				for _, l := range gologgers {
					// Verify tags, no tags means pass on everything. Right now, though.
					if len(l.tags) > 0 {
						if !matchTags(msg.Tags, l.tags) {
							continue
						}
					}

					loggers = append(loggers, l)
				}

				if len(loggers) == 0 {
					break
				}

				msg.values[ExpandMessage] = fmt.Sprintf(msg.Format, msg.Elements...)
				msg.values[ExpandSubject] = fmt.Sprintf("%v", msg.Subject)
				msg.values[ExpandMessageLevel] = msg.Level.String()

				for _, l := range loggers {
					format := l.expand(l.Format, &msg)
					l.log.Printf(format)
				}
			}
		}
	}()
}

func matchTags(msgTags, logTags []string) bool {
	for _, t := range msgTags {
		for _, tt := range logTags {
			if t == tt {
				return true
			}
		}
	}

	return false
}
