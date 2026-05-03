package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestSlogLogger(t *testing.T) {
	tests := []struct {
		name string
		run  func(Logger)
		want []string
	}{
		{
			name: "info",
			run: func(log Logger) {
				// Info は key/value を slog の構造化フィールドとして出力する。
				log.Info(context.Background(), "created", "monitorID", "123")
			},
			want: []string{
				`"level":"INFO"`,
				`"msg":"created"`,
				`"monitorID":"123"`,
			},
		},
		{
			name: "error",
			run: func(log Logger) {
				// Error は error を専用フィールドとして追加し、調査に必要な文脈を残す。
				log.Error(context.Background(), assertError("failed"), "sync failed", "target", "example.com")
			},
			want: []string{
				`"level":"ERROR"`,
				`"msg":"sync failed"`,
				`"error":"failed"`,
				`"target":"example.com"`,
			},
		},
		{
			name: "with name",
			run: func(log Logger) {
				// WithName は component 名を logger フィールドに入れ、呼び出し元を絞り込めるようにする。
				log.WithName("sakura-api").Info(context.Background(), "access")
			},
			want: []string{
				`"msg":"access"`,
				`"logger":"sakura-api"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var records bytes.Buffer
			log := NewJSONLogger(&records, slog.LevelInfo)

			tt.run(log)

			record := records.String()
			if got := strings.Split(strings.TrimSpace(record), "\n"); len(got) != 1 {
				t.Fatalf("expected one log record, got %d: %q", len(got), record)
			}
			for _, want := range tt.want {
				if !strings.Contains(record, want) {
					t.Fatalf("expected log record to contain %q, got %q", want, record)
				}
			}
		})
	}
}

type assertError string

func (e assertError) Error() string {
	return string(e)
}
