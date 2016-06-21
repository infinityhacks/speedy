package messaging

import (
	"errors"
	"fmt"

	"github.com/Sirupsen/logrus"
	"github.com/nats-io/nats"
)

// HookConf defines the vars needed to connect to nats and add the logrus hook
type HookConf struct {
	NatsConfig
	Subject    string            `json:"subject"`
	Dimensions map[string]string `json:"dimensions"`
}

// NatsHook will emit logs to the subject provided
type NatsHook struct {
	conn          *nats.Conn
	subject       string
	extraFields   map[string]string
	dynamicFields map[string]func() string
	formatter     logrus.Formatter

	LogLevels []logrus.Level
}

// AddNatsHook will connect to nats, add the hook to logrus, and percolate any errors up
func AddNatsHook(conf *HookConf) (*nats.Conn, *NatsHook, error) {
	if conf.Subject == "" {
		return nil, nil, errors.New("Must provide a subject for the nats hook")
	}

	nc, err := ConnectToNats(&conf.NatsConfig)
	if err != nil {
		return nil, nil, err
	}

	hook, err := NewNatsHook(nc, conf.Subject)
	if err != nil {
		return nil, nil, err
	}

	for k, v := range conf.Dimensions {
		hook.AddField(k, v)
	}

	logrus.AddHook(hook)

	return nc, hook, nil
}

// NewNatsHook will create a logrus hook that will automatically send
// new info into the channel
func NewNatsHook(conn *nats.Conn, subject string) (*NatsHook, error) {
	hook := NatsHook{
		conn:          conn,
		subject:       subject,
		extraFields:   make(map[string]string),
		dynamicFields: make(map[string]func() string),
		formatter:     &logrus.JSONFormatter{},
		LogLevels: []logrus.Level{
			logrus.PanicLevel,
			logrus.FatalLevel,
			logrus.ErrorLevel,
			logrus.WarnLevel,
			logrus.InfoLevel,
			logrus.DebugLevel,
		},
	}

	return &hook, nil
}

// AddField will add a simple value each emission
func (hook *NatsHook) AddField(key, value string) *NatsHook {
	hook.extraFields[key] = value
	return hook
}

// AddDynamicField will call that method on each fire
func (hook *NatsHook) AddDynamicField(key string, generator func() string) *NatsHook {
	hook.dynamicFields[key] = generator
	return hook
}

// Fire will use the connection and try to send the message to the right destination
func (hook *NatsHook) Fire(entry *logrus.Entry) error {
	if hook.conn.IsClosed() {
		return fmt.Errorf("Attempted to log on a closed connection")
	}

	// add in the new fields
	for k, v := range hook.extraFields {
		entry.Data[k] = v
	}

	for k, generator := range hook.dynamicFields {
		entry.Data[k] = generator()
	}

	bytes, err := hook.formatter.Format(entry)
	if err != nil {
		return err
	}

	return hook.conn.Publish(hook.subject, bytes)
}

// Levels will describe what levels the NatsHook is associated with
func (hook *NatsHook) Levels() []logrus.Level {
	return hook.LogLevels
}
