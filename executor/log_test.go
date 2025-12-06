package executor

import (
	"context"
	"testing"
	"time"

	"bingo/engine"
	"bingo/listener"
)

func TestLogExecutor_Type(t *testing.T) {
	exec := &LogExecutor{}
	if exec.Type() != "log" {
		t.Errorf("Type() = %v, want 'log'", exec.Type())
	}
}

func TestLogExecutor_Execute(t *testing.T) {
	exec := &LogExecutor{}

	tests := []struct {
		name    string
		action  engine.Action
		event   *listener.Event
		wantErr bool
	}{
		{
			name: "info 级别日志",
			action: engine.Action{
				Type:    "log",
				Message: "Test message: {{ .Table }}",
				Level:   "info",
			},
			event: &listener.Event{
				Table:  "test.users",
				Action: listener.ActionInsert,
			},
			wantErr: false,
		},
		{
			name: "debug 级别日志",
			action: engine.Action{
				Type:    "log",
				Message: "Debug message",
				Level:   "debug",
			},
			event: &listener.Event{
				Table:  "test.users",
				Action: listener.ActionInsert,
			},
			wantErr: false,
		},
		{
			name: "warn 级别日志",
			action: engine.Action{
				Type:    "log",
				Message: "Warning message",
				Level:   "warn",
			},
			event: &listener.Event{
				Table:  "test.users",
				Action: listener.ActionInsert,
			},
			wantErr: false,
		},
		{
			name: "error 级别日志",
			action: engine.Action{
				Type:    "log",
				Message: "Error message",
				Level:   "error",
			},
			event: &listener.Event{
				Table:  "test.users",
				Action: listener.ActionInsert,
			},
			wantErr: false,
		},
		{
			name: "默认消息",
			action: engine.Action{
				Type:  "log",
				Level: "info",
			},
			event: &listener.Event{
				Table:  "test.users",
				Action: listener.ActionInsert,
			},
			wantErr: false,
		},
		{
			name: "带字段的日志",
			action: engine.Action{
				Type:    "log",
				Message: "User ID: {{ .ID }}, Name: {{ .Name }}",
				Level:   "info",
			},
			event: &listener.Event{
				Table:  "test.users",
				Action: listener.ActionInsert,
				NewRow: map[string]interface{}{
					"id":   123,
					"name": "Test User",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := exec.Execute(ctx, tt.action, tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
