package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AXmishell/axmipic/internal/notify"
	"github.com/AXmishell/axmipic/internal/service"
)

// failingSender 始终返回错误，用于测试失败日志。
type failingSender struct{}

func (failingSender) Name() string { return "failing" }
func (failingSender) Send(context.Context, notify.Message) error {
	return errors.New("boom")
}

func TestNotifyServiceRecordsLogs(t *testing.T) {
	_, repo := newAccountService(t, true, 1<<20)
	ctx := context.Background()

	svc := service.NewNotifyService(nil, notify.NewMockSender("mock"))
	svc.SetRepository(repo)
	if err := svc.SendEmail(ctx, "a@example.com", "主题", "正文"); err != nil {
		t.Fatalf("SendEmail: %v", err)
	}
	list, err := svc.ListNotifyLogs(ctx, "email", 1, 20)
	if err != nil {
		t.Fatalf("ListNotifyLogs: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 {
		t.Fatalf("logs = %+v", list)
	}
	item := list.Items[0]
	if item.To != "a@example.com" || item.Subject != "主题" || item.Status != "sent" || item.Provider != "mock" {
		t.Fatalf("log item = %+v", item)
	}

	// 发送失败也会记录，状态为 failed 且带错误信息。
	failing := service.NewNotifyService(nil, failingSender{})
	failing.SetRepository(repo)
	if err := failing.SendEmail(ctx, "b@example.com", "s", "b"); !errors.Is(err, service.ErrNotifyFailed) {
		t.Fatalf("failing send err = %v, want ErrNotifyFailed", err)
	}
	list, err = failing.ListNotifyLogs(ctx, "email", 1, 20)
	if err != nil || list.Total != 2 {
		t.Fatalf("total = %d err=%v, want 2", list.Total, err)
	}
	foundFailed := false
	for _, it := range list.Items {
		if it.Status == "failed" && it.Error != "" {
			foundFailed = true
		}
	}
	if !foundFailed {
		t.Fatalf("failed log not recorded: %+v", list.Items)
	}

	// 非法 channel 过滤被拒绝。
	if _, err := svc.ListNotifyLogs(ctx, "bogus", 1, 20); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("invalid channel err = %v, want ErrInvalidInput", err)
	}
}
