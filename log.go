package golog

import (
	"fmt"
	"io"
	"log"
	"os"
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
	Name   string
	output io.Writer
	tags   []string
	log    *log.Logger
}

func newInstance(name string, output io.Writer, tags ...string) *Instance {
	return &Instance{
		Name:   name,
		output: output,
		tags:   tags,
	}
}

func (i *Instance) initialize() *Instance {
	i.log = log.New(os.Stdout, "", log.LstdFlags)
	return i
}

func AddLoggerInstance(name string, output io.Writer, tags ...string) {
	RegisterLogger <- newInstance(name, output, tags...).initialize()
}

type Message struct {
	Level    LogLevel
	Subject  interface{}
	Tags     []string
	Format   string
	Elements []interface{}
}

var (
	// Sink is the main channel for sending/recieving messages
	Sink chan Message
	// NewLevel is channel that can change the log level
	NewLevel chan LogLevel
	level    LogLevel = Info
	// Prefix is a channel that changes the prefix of the output
	Prefix chan string
	prefix string = ""

	// By default there is one logging instance, and it logs to stdout.
	gologgers []*Instance = []*Instance{newInstance("default", os.Stdout).initialize()}

	RegisterLogger   chan *Instance
	DeRegisterLogger chan string
)

func Log(level LogLevel, subject interface{}, format string, elements ...interface{}) {
	Sink <- Message{level, subject, nil, format, elements}
}

func IsProduction(b bool) {
	if b {
		NewLevel <- Warning
	}
}

func init() {
	Sink = make(chan Message, 256)
	NewLevel = make(chan LogLevel)
	Prefix = make(chan string)

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
			case prfx := <-Prefix:
				prefix = prfx
				for _, l := range gologgers {
					l.log = log.New(l.output, prefix, log.LstdFlags)
				}
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

				if msg.Subject != nil {
					for _, l := range loggers {
						l.log.Printf("[%v] %v, %v", msg.Level, msg.Subject, fmt.Sprintf(msg.Format, msg.Elements...))
					}
				} else {
					for _, l := range loggers {
						l.log.Printf("[%v] %v", msg.Level, fmt.Sprintf(msg.Format, msg.Elements...))
					}
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
